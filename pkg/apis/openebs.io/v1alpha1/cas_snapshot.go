/*
Copyright 2017 The OpenEBS Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CASSnapshot represents a cas snapshot
type CASSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec i.e. specifications of this cas snapshot
	Spec SnapshotSpec `json:"spec"`
}

// SnapshotSpec has the properties of a cas snapshot
type SnapshotSpec struct {
	CasType    string `json:"casType"`
	VolumeName string `json:"volumeName"`
}

// SnapshotListOptions has the properties of a cas snapshot list
type SnapshotListOptions struct {
	CasType    string `json:"casType,omitempty"`
	VolumeName string `json:"volumeName,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}

// CASSnapshotList is a list of CASSnapshot resources
type CASSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	// Options will contain the volume name and cas type for which snapshots is listed
	Options SnapshotListOptions
	// Items are the list of volumes
	Items []CASSnapshot `json:"items"`
}
