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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baremetalcontrollerv1 "github.com/Unbounder1/bare-metal-controller/api/v1"
	"github.com/Unbounder1/bare-metal-controller/internal/power"
)

// ServerReconciler reconciles a Server object
type ServerReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	WolSender  power.WolSender
	SSHClient  power.SSHClient
	IPMIClient power.IPMIClient
	Pinger     power.Pinger
}

func (r *ServerReconciler) powerOn(server *baremetalcontrollerv1.Server) error {
	switch server.Spec.Type {
	case baremetalcontrollerv1.ControlTypeWOL:
		if server.Spec.Control.WOL == nil {
			return fmt.Errorf("WOL config is required")
		}
		if server.Spec.Control.WOL.MACAddress == "" {
			return fmt.Errorf("WOL MAC address is required")
		}
		return r.WolSender.Wake(server.Spec.Control.WOL.MACAddress, server.Spec.Control.WOL.Port)

	case baremetalcontrollerv1.ControlTypeIPMI:
		if server.Spec.Control.IPMI == nil {
			return fmt.Errorf("IPMI config is required")
		}
		if server.Spec.Control.IPMI.Address == "" {
			return fmt.Errorf("IPMI address is required")
		}
		if server.Spec.Control.IPMI.Username == "" || server.Spec.Control.IPMI.Password == "" {
			return fmt.Errorf("IPMI username and password are required")
		}
		return r.IPMIClient.PowerOn(server.Spec.Control.IPMI.Address, server.Spec.Control.IPMI.Username, server.Spec.Control.IPMI.Password)

	default:
		return fmt.Errorf("unknown control type: %s", server.Spec.Type)
	}
}

func (r *ServerReconciler) powerOff(server *baremetalcontrollerv1.Server) error {
	switch server.Spec.Type {
	case baremetalcontrollerv1.ControlTypeWOL:
		if server.Spec.Control.WOL == nil {
			return fmt.Errorf("WOL config is required")
		}
		if server.Spec.Control.WOL.Address == "" {
			return fmt.Errorf("WOL address is required")
		}
		return r.SSHClient.Shutdown(server.Spec.Control.WOL.Address, server.Spec.Control.WOL.User)

	case baremetalcontrollerv1.ControlTypeIPMI:
		if server.Spec.Control.IPMI == nil {
			return fmt.Errorf("IPMI config is required")
		}
		if server.Spec.Control.IPMI.Address == "" {
			return fmt.Errorf("IPMI address is required")
		}
		if server.Spec.Control.IPMI.Username == "" || server.Spec.Control.IPMI.Password == "" {
			return fmt.Errorf("IPMI username and password are required")
		}
		return r.IPMIClient.PowerOff(server.Spec.Control.IPMI.Address, server.Spec.Control.IPMI.Username, server.Spec.Control.IPMI.Password)

	default:
		return fmt.Errorf("unknown control type: %s", server.Spec.Type)
	}
}

// +kubebuilder:rbac:groups=bare-metal-controller.bare-metal.io,resources=servers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bare-metal-controller.bare-metal.io,resources=servers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=bare-metal-controller.bare-metal.io,resources=servers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Server object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *ServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var server baremetalcontrollerv1.Server
	if err := r.Get(ctx, req.NamespacedName, &server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Set default PowerState to "off" if not specified
	if server.Spec.PowerState == "" {
		server.Spec.PowerState = baremetalcontrollerv1.PowerStateOff
	}

	// Ignore if failed status
	if server.Status.Status == baremetalcontrollerv1.StatusFailed {
		return ctrl.Result{}, nil
	}

	// Set to failed if failure count exceeds threshold
	if server.Status.FailureCount >= 5 {
		server.Status.Status = baremetalcontrollerv1.StatusFailed
		r.Status().Update(ctx, &server)
		return ctrl.Result{}, nil
	}

	// If pending, check reachability to update status
	if server.Status.Status == baremetalcontrollerv1.StatusPending ||
		server.Status.Status == baremetalcontrollerv1.StatusDraining {

		reachable := r.Pinger.IsReachable(server.Spec.Control.WOL.Address)

		var targetReached bool
		var newStatus baremetalcontrollerv1.CurrentStatus

		switch server.Status.Status {
		case baremetalcontrollerv1.StatusPending:
			targetReached = reachable
			newStatus = baremetalcontrollerv1.StatusActive
		case baremetalcontrollerv1.StatusDraining:
			targetReached = !reachable
			newStatus = baremetalcontrollerv1.StatusOffline
		}

		if targetReached {
			server.Status.Status = newStatus
			server.Status.FailingSince = nil
			server.Status.FailureCount = 0
			server.Status.Message = ""
		} else {
			if server.Status.FailingSince == nil {
				now := metav1.Now()
				server.Status.FailingSince = &now
			}
			server.Status.FailureCount++
		}

		r.Status().Update(ctx, &server)
		if targetReached {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// Check current state
	currentState := baremetalcontrollerv1.PowerStateOff
	if server.Status.Status == baremetalcontrollerv1.StatusActive || server.Status.Status == baremetalcontrollerv1.StatusPending {
		currentState = baremetalcontrollerv1.PowerStateOn
	}

	// If desired state matches current state, do nothing
	if server.Spec.PowerState == currentState {
		return ctrl.Result{}, nil
	}

	// Perform power action
	var senderStatus error

	switch server.Spec.PowerState {
	case baremetalcontrollerv1.PowerStateOn:
		senderStatus = r.powerOn(&server)
	case baremetalcontrollerv1.PowerStateOff:
		senderStatus = r.powerOff(&server)
	default:
		return ctrl.Result{}, nil
	}

	if senderStatus != nil {
		server.Status.Status = baremetalcontrollerv1.StatusFailed
		server.Status.Message = fmt.Sprintf("Power action failed: %v", senderStatus)
		r.Status().Update(ctx, &server)
		return ctrl.Result{}, senderStatus
	}

	server.Status.Status = baremetalcontrollerv1.StatusPending
	server.Status.Message = ""
	r.Status().Update(ctx, &server)
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&baremetalcontrollerv1.Server{}).
		Named("server").
		Complete(r)
}
