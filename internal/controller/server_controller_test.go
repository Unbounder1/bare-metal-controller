// internal/controller/server_controller_test.go
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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	baremetalcontrollerv1 "github.com/Unbounder1/bare-metal-controller/api/v1"
	"github.com/Unbounder1/bare-metal-controller/internal/power"
)

/*
> Assuming controller can reach every server via LAN.
> Assuming controller can passwordless authenticate to servers.
Server Reconciliation workflow
 - Event triggers server reconcillation
    - checks if this server exists as a resource
        - no: error, exit
    - checks if status matches declared
	    - yes: nil, exit
	- checks if status is pending state: provisioning, etc, have some time timeout for this
	- check type
		- Wol:
			- Turning on the server -> send mac address packet
			- Turning off the server -> ssh, send shutdown command
		- ipmi:
			- TBD
	- Reconcile based on type definition
	- Update status correctly (error if anything goes wrong, otherwise offline/online/provisioning)
		- if turning on:
			- check if it is reachable to confirm power on, then set status accordingly
		- if turning of:
			- sanity check ping to confirm power
*/

var _ = Describe("Server Controller", func() {

	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		ctx        context.Context
		reconciler *ServerReconciler
		mockWol    *power.MockWolSender
		mockSSH    *power.MockSSHClient
		mockPinger *power.MockPinger
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockWol = &power.MockWolSender{}
		mockSSH = &power.MockSSHClient{}
		mockPinger = &power.MockPinger{}

		reconciler = &ServerReconciler{
			Client:    k8sClient,
			Scheme:    k8sClient.Scheme(),
			WolSender: mockWol,
			SSHClient: mockSSH,
			Pinger:    mockPinger,
		}
	})

	// Helper function to create a WoL server
	createWolServer := func(name string, desiredPower baremetalcontrollerv1.PowerState) *baremetalcontrollerv1.Server {
		return &baremetalcontrollerv1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: baremetalcontrollerv1.ServerSpec{
				PowerState: desiredPower,
				Type:       baremetalcontrollerv1.ControlTypeWOL,
				Control: baremetalcontrollerv1.ControlSpecs{
					WOL: &baremetalcontrollerv1.WOLSpecs{
						Address:    "192.168.1.100",
						MACAddress: "00:11:22:33:44:55",
						Port:       9,
						User:       "admin",
					},
				},
			},
		}
	}

	// Helper function to create an IPMI server
	createIPMIServer := func(name string, desiredPower baremetalcontrollerv1.PowerState) *baremetalcontrollerv1.Server {
		return &baremetalcontrollerv1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: baremetalcontrollerv1.ServerSpec{
				PowerState: desiredPower,
				Type:       baremetalcontrollerv1.ControlTypeIPMI,
				Control: baremetalcontrollerv1.ControlSpecs{
					IPMI: &baremetalcontrollerv1.IPMISpecs{
						Address:  "192.168.1.101",
						Username: "admin",
						Password: "password",
					},
				},
			},
		}
	}

	// Helper to clean up a server resource
	deleteServer := func(name string) {
		server := &baremetalcontrollerv1.Server{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, server)
		if err == nil {
			Expect(k8sClient.Delete(ctx, server)).To(Succeed())
		}
	}

	Context("When server resource does not exist", func() {
		It("should return without error", func() {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "non-existent-server"},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a WoL server", func() {
		const serverName = "wol-test-server"

		AfterEach(func() {
			deleteServer(serverName)
		})

		Context("when turning on the server", func() {
			BeforeEach(func() {
				server := createWolServer(serverName, baremetalcontrollerv1.PowerStateOn)
				Expect(k8sClient.Create(ctx, server)).To(Succeed())
			})

			It("should send WoL magic packet with correct parameters", func() {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(mockWol.WakeCalled).To(BeTrue())
				Expect(mockWol.LastMAC).To(Equal("00:11:22:33:44:55"))
				Expect(mockWol.LastPort).To(Equal(9))
			})

			It("should set status to booting after sending WoL packet", func() {
				mockPinger.Reachable = false

				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())

				var server baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &server)).To(Succeed())
				Expect(server.Status.Status).To(Equal(baremetalcontrollerv1.StatusPending))
			})

			It("should set status to active when server becomes reachable", func() {
				mockPinger.Reachable = true

				for i := 0; i < 5; i++ {
					_, err := reconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: serverName},
					})
					Expect(err).NotTo(HaveOccurred())
				}

				var server baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &server)).To(Succeed())
				Expect(server.Status.Status).To(Equal(baremetalcontrollerv1.StatusActive))
			})

			It("should set status to failed when WoL packet fails to send", func() {
				mockWol.ReturnError = errors.NewServiceUnavailable("network error")

				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).To(HaveOccurred())

				var server baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &server)).To(Succeed())
				Expect(server.Status.Status).To(Equal(baremetalcontrollerv1.StatusFailed))
			})
		})

		Context("when turning off the server", func() {
			BeforeEach(func() {
				server := createWolServer(serverName, baremetalcontrollerv1.PowerStateOff)
				Expect(k8sClient.Create(ctx, server)).To(Succeed())

				// Set initial status to active
				var created baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &created)).To(Succeed())
				created.Status.Status = baremetalcontrollerv1.StatusActive
				Expect(k8sClient.Status().Update(ctx, &created)).To(Succeed())
			})

			It("should send SSH shutdown command", func() {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(mockSSH.ShutdownCalled).To(BeTrue())
				Expect(mockSSH.LastHost).To(Equal("192.168.1.100"))
				Expect(mockSSH.LastUser).To(Equal("admin"))
			})

			It("should set status to draining after sending shutdown", func() {
				mockPinger.Reachable = true // Still reachable during shutdown

				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())

				var server baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &server)).To(Succeed())
				Expect(server.Status.Status).To(Equal(baremetalcontrollerv1.StatusDraining))
			})

			It("should set status to offline when server is unreachable", func() {
				mockPinger.Reachable = false

				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())

				var server baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &server)).To(Succeed())
				Expect(server.Status.Status).To(Equal(baremetalcontrollerv1.StatusOffline))
			})

			It("should set status to failed when SSH command fails", func() {
				mockSSH.ReturnError = errors.NewServiceUnavailable("connection refused")

				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).To(HaveOccurred())

				var server baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &server)).To(Succeed())
				Expect(server.Status.Status).To(Equal(baremetalcontrollerv1.StatusFailed))
			})
		})

		Context("when status already matches desired state", func() {
			It("should not send any commands when already active and desired is on", func() {
				server := createWolServer(serverName, baremetalcontrollerv1.PowerStateOn)
				Expect(k8sClient.Create(ctx, server)).To(Succeed())

				var created baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &created)).To(Succeed())
				created.Status.Status = baremetalcontrollerv1.StatusActive
				Expect(k8sClient.Status().Update(ctx, &created)).To(Succeed())

				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(mockWol.WakeCalled).To(BeFalse())
				Expect(mockSSH.ShutdownCalled).To(BeFalse())
			})

			It("should not send any commands when already offline and desired is off", func() {
				server := createWolServer(serverName, baremetalcontrollerv1.PowerStateOff)
				Expect(k8sClient.Create(ctx, server)).To(Succeed())

				var created baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &created)).To(Succeed())
				created.Status.Status = baremetalcontrollerv1.StatusOffline
				Expect(k8sClient.Status().Update(ctx, &created)).To(Succeed())

				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(mockWol.WakeCalled).To(BeFalse())
				Expect(mockSSH.ShutdownCalled).To(BeFalse())
			})
		})
	})

	Context("When reconciling an IPMI server", func() {
		const serverName = "ipmi-test-server"

		var mockIPMI *power.MockIPMIClient

		BeforeEach(func() {
			mockIPMI = &power.MockIPMIClient{}
			reconciler.IPMIClient = mockIPMI
		})

		AfterEach(func() {
			deleteServer(serverName)
		})

		Context("when turning on the server", func() {
			BeforeEach(func() {
				server := createIPMIServer(serverName, baremetalcontrollerv1.PowerStateOn)
				Expect(k8sClient.Create(ctx, server)).To(Succeed())
			})

			It("should send IPMI power on command", func() {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(mockIPMI.PowerOnCalled).To(BeTrue())
				Expect(mockIPMI.LastAddress).To(Equal("192.168.1.101"))
			})

			It("should set status to active when server is reachable", func() {
				mockPinger.Reachable = true

				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())

				var server baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &server)).To(Succeed())
				Expect(server.Status.Status).To(Equal(baremetalcontrollerv1.StatusActive))
			})
		})

		Context("when turning off the server", func() {
			BeforeEach(func() {
				server := createIPMIServer(serverName, baremetalcontrollerv1.PowerStateOff)
				Expect(k8sClient.Create(ctx, server)).To(Succeed())

				var created baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &created)).To(Succeed())
				created.Status.Status = baremetalcontrollerv1.StatusActive
				Expect(k8sClient.Status().Update(ctx, &created)).To(Succeed())
			})

			It("should send IPMI power off command", func() {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(mockIPMI.PowerOffCalled).To(BeTrue())
			})

			It("should set status to offline when server is unreachable", func() {
				mockPinger.Reachable = false

				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: serverName},
				})

				Expect(err).NotTo(HaveOccurred())

				var server baremetalcontrollerv1.Server
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &server)).To(Succeed())
				Expect(server.Status.Status).To(Equal(baremetalcontrollerv1.StatusOffline))
			})
		})
	})

	Context("When handling pending states", func() {
		const serverName = "pending-test-server"

		AfterEach(func() {
			deleteServer(serverName)
		})

		It("should requeue when status is booting", func() {
			server := createWolServer(serverName, baremetalcontrollerv1.PowerStateOn)
			Expect(k8sClient.Create(ctx, server)).To(Succeed())

			var created baremetalcontrollerv1.Server
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &created)).To(Succeed())
			created.Status.Status = baremetalcontrollerv1.StatusPending
			Expect(k8sClient.Status().Update(ctx, &created)).To(Succeed())

			mockPinger.Reachable = false

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: serverName},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("should requeue when status is draining", func() {
			server := createWolServer(serverName, baremetalcontrollerv1.PowerStateOff)
			Expect(k8sClient.Create(ctx, server)).To(Succeed())

			var created baremetalcontrollerv1.Server
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &created)).To(Succeed())
			created.Status.Status = baremetalcontrollerv1.StatusDraining
			Expect(k8sClient.Status().Update(ctx, &created)).To(Succeed())

			mockPinger.Reachable = true

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: serverName},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("should transition from booting to active when reachable", func() {
			server := createWolServer(serverName, baremetalcontrollerv1.PowerStateOn)
			Expect(k8sClient.Create(ctx, server)).To(Succeed())

			var created baremetalcontrollerv1.Server
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &created)).To(Succeed())
			created.Status.Status = baremetalcontrollerv1.StatusPending
			Expect(k8sClient.Status().Update(ctx, &created)).To(Succeed())

			mockPinger.Reachable = true

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: serverName},
			})

			Expect(err).NotTo(HaveOccurred())

			var updated baremetalcontrollerv1.Server
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &updated)).To(Succeed())
			Expect(updated.Status.Status).To(Equal(baremetalcontrollerv1.StatusActive))
		})

		It("should transition from draining to offline when unreachable", func() {
			server := createWolServer(serverName, baremetalcontrollerv1.PowerStateOff)
			Expect(k8sClient.Create(ctx, server)).To(Succeed())

			var created baremetalcontrollerv1.Server
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &created)).To(Succeed())
			created.Status.Status = baremetalcontrollerv1.StatusDraining
			Expect(k8sClient.Status().Update(ctx, &created)).To(Succeed())

			mockPinger.Reachable = false

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: serverName},
			})

			Expect(err).NotTo(HaveOccurred())

			var updated baremetalcontrollerv1.Server
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: serverName}, &updated)).To(Succeed())
			Expect(updated.Status.Status).To(Equal(baremetalcontrollerv1.StatusOffline))
		})
	})

	Context("When validating server specs", func() {
		const serverName = "validation-test-server"

		AfterEach(func() {
			deleteServer(serverName)
		})

		It("should fail when WoL server has no MAC address", func() {
			server := &baremetalcontrollerv1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name: serverName,
				},
				Spec: baremetalcontrollerv1.ServerSpec{
					PowerState: baremetalcontrollerv1.PowerStateOn,
					Type:       baremetalcontrollerv1.ControlTypeWOL,
					Control: baremetalcontrollerv1.ControlSpecs{
						WOL: &baremetalcontrollerv1.WOLSpecs{
							Address: "192.168.1.100",
							// MACAddress missing
						},
					},
				},
			}
			err := k8sClient.Create(ctx, server)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("macAddress"))
		})

		It("should fail when type is WoL but WoL specs are nil", func() {
			server := &baremetalcontrollerv1.Server{
				ObjectMeta: metav1.ObjectMeta{
					Name: serverName,
				},
				Spec: baremetalcontrollerv1.ServerSpec{
					PowerState: baremetalcontrollerv1.PowerStateOn,
					Type:       baremetalcontrollerv1.ControlTypeWOL,
					Control:    baremetalcontrollerv1.ControlSpecs{
						// WOL is nil
					},
				},
			}
			Expect(k8sClient.Create(ctx, server)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: serverName},
			})

			Expect(err).To(HaveOccurred())
		})
	})
})
