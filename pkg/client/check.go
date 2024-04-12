/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/hydrophone/pkg/common"
	"sigs.k8s.io/hydrophone/pkg/log"

	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// Contains all the necessary channels to transfer data
type streamLogs struct {
	logCh  chan string
	errCh  chan error
	doneCh chan bool
}

// PrintE2ELogs checks for Pod and start a go routine if new deployment added
func (c *Client) PrintE2ELogs(ctx context.Context) error {
	informerFactory := informers.NewSharedInformerFactory(c.ClientSet, 10*time.Second)

	podInformer := informerFactory.Core().V1().Pods()

	if _, err := podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{}); err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	informerFactory.Start(wait.NeverStop)
	informerFactory.WaitForCacheSync(wait.NeverStop)

	for {
		pod, _ := podInformer.Lister().Pods(viper.GetString("namespace")).Get(common.PodName)
		if pod.Status.Phase == v1.PodRunning {
			var err error
			stream := streamLogs{
				logCh:  make(chan string),
				errCh:  make(chan error),
				doneCh: make(chan bool),
			}

			go getPodLogs(ctx, c.ClientSet, stream)

		loop:
			for {
				select {
				case err = <-stream.errCh:
					log.Fatal(err)
				case logStream := <-stream.logCh:
					_, err = fmt.Print(logStream)
					if err != nil {
						log.Fatal(err)
					}
				case <-stream.doneCh:
					break loop
				}
			}
			break
		}
	}

	return nil
}

// FetchExitCode waits for pod to be in terminated state and get the exit code
func (c *Client) FetchExitCode(ctx context.Context) error {
	// Watching the pod's status
	watchInterface, err := c.ClientSet.CoreV1().Pods(viper.GetString("namespace")).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", common.PodName),
	})
	if err != nil {
		return fmt.Errorf("failed to watch Pods: %w", err)
	}

	log.Println("Waiting for Pod to terminate...")
	for event := range watchInterface.ResultChan() {
		pod, ok := event.Object.(*v1.Pod)
		if !ok {
			log.Printf("Received unexpected %T object from Watch.", pod)
			c.ExitCode = -1
			return nil
		}

		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			log.Println("Pod terminated.")
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.Name == common.ConformanceContainer && containerStatus.State.Terminated != nil {
					c.ExitCode = int(containerStatus.State.Terminated.ExitCode)
				}
			}
			break
		} else if pod.Status.Phase == v1.PodRunning {
			terminated := false
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.State.Terminated != nil {
					terminated = true
					log.Printf("Container %s terminated.", containerStatus.Name)
					if containerStatus.Name == common.ConformanceContainer {
						c.ExitCode = int(containerStatus.State.Terminated.ExitCode)
					}
				}
			}
			if terminated {
				break
			}
		}
	}

	return nil
}
