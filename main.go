package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/net/websocket"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Config struct {
	KubeconfigPath       string `yaml:"kubeconfig_path"`
	NamespacePrefix      string `yaml:"namespace_prefix"`
	LokiAddress          string `yaml:"loki_address"`
	LokiWebSocketAddress string `yaml:"loki_websocket_address"`
	DelayFor             int    `yaml:"delay_for"`
	PollInterval         int    `yaml:"poll_interval"`
}

type LokiQueryTailResponse struct {
	Streams []struct {
		Stream map[string]string `json:"stream"`
		Values [][]interface{}   `json:"values"`
	} `json:"streams"`
	DroppedEntries []struct {
		Labels    map[string]string `json:"labels"`
		Timestamp string            `json:"timestamp"`
	} `json:"dropped_entries"`
}

type PodInfo struct {
	Namespace string
	PodName   string
	StartTime time.Time
}

type JobQueue struct {
	podInfoQueue []PodInfo
	loggedPods   map[string]bool
}

func (jq *JobQueue) AddPodToQueue(podInfo PodInfo) {
	jq.podInfoQueue = append(jq.podInfoQueue, podInfo)
}

func (jq *JobQueue) GetPodFromQueue() (PodInfo, bool) {
	if len(jq.podInfoQueue) == 0 {
		return PodInfo{}, false
	}
	podInfo := jq.podInfoQueue[0]
	jq.podInfoQueue = jq.podInfoQueue[1:]
	return podInfo, true
}

func (jq *JobQueue) MarkPodAsLogged(podInfo PodInfo) {
	jq.loggedPods[fmt.Sprintf("%s/%s", podInfo.Namespace, podInfo.PodName)] = true
}

func (jq *JobQueue) IsPodLogged(podInfo PodInfo) bool {
	_, ok := jq.loggedPods[fmt.Sprintf("%s/%s", podInfo.Namespace, podInfo.PodName)]
	return ok
}

func getTailLogsFromLoki(podInfo PodInfo, lokiAddress, LokiWebsocketAddress string, delayFor int) error {
	startedAt := podInfo.StartTime.UnixNano()

	query := fmt.Sprintf(`{pod_name="%s"}`, podInfo.PodName)

	u, err := url.Parse(fmt.Sprintf("%s/loki/api/v1/tail", LokiWebsocketAddress))
	if err != nil {
		return err
	}

	params := u.Query()
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(startedAt, 10))
	params.Set("delay_for", strconv.Itoa(delayFor))
	u.RawQuery = params.Encode()

	config, err := websocket.NewConfig(u.String(), lokiAddress)
	if err != nil {
		return err
	}
	config.Header.Set("X-Scope-OrgID", podInfo.Namespace)

	ws, err := websocket.DialConfig(config)
	if err != nil {
		return err
	}
	defer ws.Close()

	var lokiResp LokiQueryTailResponse
	err = websocket.JSON.Receive(ws, &lokiResp)
	if err != nil {
		return err
	}

	if len(lokiResp.Streams) > 0 {
		now := time.Now()
		timeDiff := now.Sub(podInfo.StartTime)
		log.Printf("First log line for pod %s in namespace %s: (Time difference: %s)", podInfo.PodName, podInfo.Namespace, timeDiff)
		return nil
	}

	return fmt.Errorf("no logs found for pod %s", podInfo.PodName)
}

func main() {
	configFile := "config.yaml"

	configFileData, err := os.Open(configFile)
	if err != nil {
		log.Fatalf("Failed to open config file: %v", err)
	}
	defer configFileData.Close()

	var config Config
	decoder := yaml.NewDecoder(configFileData)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	if config.NamespacePrefix == "" {
		config.NamespacePrefix = "logger-ns"
	}

	if config.KubeconfigPath == "" {
		config.KubeconfigPath = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", config.KubeconfigPath)
	if err != nil {
		log.Fatalf("Error building kubeconfig from %s: %v", config.KubeconfigPath, err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	jobQueue := JobQueue{
		loggedPods: make(map[string]bool),
	}

	for {
		namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), v1.ListOptions{})
		if err != nil {
			log.Fatalf("Failed to list namespaces: %v", err)
		}

		for _, namespace := range namespaces.Items {
			if !isTargetNamespace(namespace.Name, config.NamespacePrefix) {
				continue
			}

			pods, err := clientset.CoreV1().Pods(namespace.Name).List(context.TODO(), v1.ListOptions{})
			if err != nil {
				log.Fatalf("Failed to list pods in namespace %s: %v", namespace.Name, err)
			}

			for _, pod := range pods.Items {
				if pod.Status.StartTime != nil && !jobQueue.IsPodLogged(PodInfo{
					Namespace: namespace.Name,
					PodName:   pod.Name,
					StartTime: pod.Status.StartTime.Time,
				}) {
					podInfo := PodInfo{
						Namespace: namespace.Name,
						PodName:   pod.Name,
						StartTime: pod.Status.StartTime.Time,
					}
					jobQueue.AddPodToQueue(podInfo)
				}
			}
		}

		for {
			podInfo, ok := jobQueue.GetPodFromQueue()
			if !ok {
				break
			}

			err := getTailLogsFromLoki(podInfo, config.LokiAddress, config.LokiWebSocketAddress, config.DelayFor)
			if err != nil {
				log.Printf("Error getting logs for pod %s in namespace %s: %v", podInfo.PodName, podInfo.Namespace, err)
			} else {
				jobQueue.MarkPodAsLogged(podInfo)
			}
		}

		time.Sleep(time.Duration(config.PollInterval) * time.Second)
	}
}

func isTargetNamespace(namespaceName, namespacePrefix string) bool {
	return len(namespaceName) >= len(namespacePrefix) && namespaceName[:len(namespacePrefix)] == namespacePrefix
}
