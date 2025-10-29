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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	jarvisiov1 "github.com/motilayo/jarvis/controller/api/v1"

	grpcClient "github.com/motilayo/jarvis/controller/client"

	"golang.org/x/sync/errgroup"
)

// CommandReconciler reconciles a Command object
type CommandReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

var finalizer = "jarvis.io/finalizer"

// +kubebuilder:rbac:groups=jarvis.io,resources=commands,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=jarvis.io,resources=commands/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=jarvis.io,resources=commands/finalizers,verbs=update
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services;endpoints;nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
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

	if cmd.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(cmd, finalizer) {
		controllerutil.RemoveFinalizer(cmd, finalizer)
		if err := r.Update(ctx, cmd); err != nil {
			return ctrl.Result{}, err
		}
	}

	if !controllerutil.ContainsFinalizer(cmd, finalizer) {
		controllerutil.AddFinalizer(cmd, finalizer)
		if err := r.Update(ctx, cmd); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info("Reconciling Command", "name", cmd.Name, "namespace", cmd.Namespace, "command", cmd.Spec.Command)

	// Step 1: Get all nodes in the cluster
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		log.Error(err, "Failed to list nodes")
		return ctrl.Result{}, err
	}

	sliceList := &discoveryv1.EndpointSliceList{}
	if err := r.List(ctx, sliceList,
		client.InNamespace("jarvis"),
		client.MatchingLabels{"kubernetes.io/service-name": "jarvis-agent"},
	); err != nil {
		log.Error(err, "failed to list EndpointSlices for jarvis-agent")
		return ctrl.Result{}, err
	}

	nodeIP := map[string]string{}
	for _, slice := range sliceList.Items {
		for _, ep := range slice.Endpoints {
			if ep.NodeName != nil && len(ep.Addresses) > 0 {
				nodeIP[*ep.NodeName] = ep.Addresses[0]
			}
		}
	}

	selector := cmd.Spec.Selector

	type target struct {
		node string
		ip   string
	}
	var targets []target
	for _, node := range nodeList.Items {
		if len(selector.NodeSelectorTerms) > 0 && !matchNodeSelector(&node, selector) {
			continue
		}

		ip, ok := nodeIP[node.Name]
		if !ok || ip == "" {
			eventName := fmt.Sprintf("%s-%s", cmd.Name, node.Name)
			msg := fmt.Sprintf("Agent not found for node %s (skipping)", node.Name)
			r.Recorder.Event(cmd, corev1.EventTypeWarning, eventName, msg)
			continue
		}
		targets = append(targets, target{node: node.Name, ip: ip})
	}

	commandName := cmd.Name
	commandStr := cmd.Spec.Command

	go func() {
		g, gctx := errgroup.WithContext(context.Background())
		for _, target := range targets {
			nodeName := target.node
			ip := target.ip

			// Fire one goroutine per node via errgroup
			g.Go(func() error {
				output, err := grpcClient.RunCommandOnNode(gctx, ip, nodeName, commandStr)
				eventName := fmt.Sprintf("%s-%s", commandName, nodeName)
				if err != nil {
					msg := fmt.Sprintf("Failed on %s: %v", nodeName, err)
					r.Recorder.Event(cmd, corev1.EventTypeWarning, eventName, msg)
					return err
				}
				r.Recorder.Event(cmd, corev1.EventTypeNormal, eventName, output)
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			log.Error(err, "one or more node executions failed")
		} else {
			log.Info("all node executions completed")
		}
	}()

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
		Watches(&discoveryv1.EndpointSlice{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				if obj.GetName() == "jarvis-agent" {
					return []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{
								Name:      obj.GetName(),
								Namespace: obj.GetNamespace(),
							},
						},
					}
				}
				return []reconcile.Request{}
			},
			)).
		Complete(r)
}
