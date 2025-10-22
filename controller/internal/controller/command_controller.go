/*
Copyright 2025.

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

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	jarvisiov1 "github.com/motilayo/jarvis/controller/api/v1"

	agentclient "github.com/motilayo/jarvis/controller/client"
)

// CommandReconciler reconciles a Command object
type CommandReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=jarvis.io,resources=commands,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=jarvis.io,resources=commands/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=jarvis.io,resources=commands/finalizers,verbs=update
func (r *CommandReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	cmd := &jarvisiov1.Command{}

	if err := r.Get(ctx, req.NamespacedName, cmd); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch Command")
		return ctrl.Result{}, err
	}

	log.Info("Reconciling Command", "name", cmd.Name, "namespace", cmd.Namespace, "command", cmd.Spec.Command)

	// Step 1: Get all nodes in the cluster
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		log.Error(err, "Failed to list nodes")
		return ctrl.Result{}, err
	}

	selector := cmd.Spec.Selector
	for _, node := range nodeList.Items {
		if !matchNodeSelector(&node, selector) {
			continue
		}

		var nodeIP string
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodeIP = addr.Address
				break
			}
		}
		if nodeIP == "" {
			log.Info("No InternalIP found for node", "node", node.Name)
			continue
		}

		// Run command concurrently on each node
		go func(nodeName, nodeIP string) {
			// The agentclient.RunCommandOnNode helper is assumed to be available in the project
			output, err := agentclient.RunCommandOnNode(ctx, nodeIP, cmd.Spec.Command)
			if err != nil {
				log.Error(err, "Command execution failed", "node", nodeName)
				r.Recorder.Eventf(cmd, corev1.EventTypeWarning, "CommandFailed",
					"Failed to execute command on node %s: %v", nodeName, err)
				return
			}

			log.Info("Command executed successfully", "node", nodeName, "output", output)
			r.Recorder.Eventf(cmd, corev1.EventTypeNormal, "CommandResult",
				"Node: %s | Output: %s", nodeName, output)
		}(node.Name, nodeIP)
	}

	return ctrl.Result{}, nil
}

func matchNodeSelector(node *corev1.Node, selector corev1.NodeSelector) bool {
	labelsMap := labels.Set(node.Labels)

	for _, term := range selector.NodeSelectorTerms {
		matchedTerm := true
		for _, expr := range term.MatchExpressions {
			switch expr.Operator {
			case corev1.NodeSelectorOpIn:
				if !labelsMap.Has(expr.Key) || !contains(expr.Values, labelsMap[expr.Key]) {
					matchedTerm = false
				}
			case corev1.NodeSelectorOpNotIn:
				if labelsMap.Has(expr.Key) && contains(expr.Values, labelsMap[expr.Key]) {
					matchedTerm = false
				}
			case corev1.NodeSelectorOpExists:
				if !labelsMap.Has(expr.Key) {
					matchedTerm = false
				}
			case corev1.NodeSelectorOpDoesNotExist:
				if labelsMap.Has(expr.Key) {
					matchedTerm = false
				}
			}
		}
		if matchedTerm {
			return true // Node matches at least one term
		}
	}
	return false
}

func contains(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *CommandReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("command-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&jarvisiov1.Command{}).
		Named("command").
		Complete(r)
}
