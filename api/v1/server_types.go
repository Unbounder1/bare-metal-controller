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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServerSpec defines the desired state of Server.
type ServerSpec struct {
	// +kubebuilder:validation:Enum=on;off
	PowerState PowerState   `json:"powerState"`
	Type       ControlType  `json:"type,omitempty"`
	Control    ControlSpecs `json:"control,omitempty"`
}

type PowerState string

const (
	PowerStateOn  PowerState = "on"
	PowerStateOff PowerState = "off"
)

// +kubebuilder:validation:Enum=wol;ipmi
type ControlType string

const (
	ControlTypeWOL  ControlType = "wol"
	ControlTypeIPMI ControlType = "ipmi"
)

type ControlSpecs struct {
	IPMI *IPMISpecs `json:"ipmi,omitempty"`
	WOL  *WOLSpecs  `json:"wol,omitempty"`
}

type IPMISpecs struct {
	// +kubebuilder:validation:Required
	Address  string `json:"address,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type WOLSpecs struct {
	// +kubebuilder:validation:Required
	Address string `json:"address,omitempty"`
	// +kubebuilder:validation:Required
	MACAddress string `json:"macAddress,omitempty"`

	// +kubebuilder:default=9
	Port int    `json:"port,omitempty"`
	User string `json:"user,omitempty"`
}

// ServerStatus defines the observed state of Server.
type ServerStatus struct {
	Status CurrentStatus `json:"status,omitempty"`

	// +optional
	Message string `json:"message,omitempty"`

	// +optional
	FailingSince *metav1.Time `json:"failingSince,omitempty"`

	// +optional
	FailureCount int `json:"failureCount,omitempty"`
}

type CurrentStatus string

const (
	StatusPending  CurrentStatus = "pending"
	StatusActive   CurrentStatus = "active"
	StatusOffline  CurrentStatus = "offline"
	StatusDraining CurrentStatus = "draining"
	StatusFailed   CurrentStatus = "failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// Server is the Schema for the servers API.
type Server struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerSpec   `json:"spec,omitempty"`
	Status ServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServerList contains a list of Server.
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Server `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}
