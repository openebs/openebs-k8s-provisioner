/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/golang/glog"
	crdv1 "github.com/openebs/openebs-k8s-provisioner/pkg/apis/crd/v1"
	crdclient "github.com/openebs/openebs-k8s-provisioner/pkg/client"
	"github.com/openebs/openebs-k8s-provisioner/pkg/volume"
	"github.com/openebs/openebs-k8s-provisioner/pkg/volume/gluster"
	"github.com/openebs/openebs-k8s-provisioner/pkg/volume/hostpath"
	"github.com/openebs/openebs-k8s-provisioner/pkg/volume/openebs"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/v7/controller"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	provisionerName  = "volumesnapshot.external-storage.k8s.io/snapshot-promoter"
	provisionerIDAnn = "snapshotProvisionerIdentity"
	// LeaderElectionKey represents ENV for disable/enable leaderElection for
	// snapshot-provisioner
	LeaderElectionKey = "LEADER_ELECTION_ENABLED"
)

type snapshotProvisioner struct {
	// Kubernetes Client.
	client kubernetes.Interface
	// CRD client
	crdclient *rest.RESTClient
	// Identity of this snapshotProvisioner, generated. Used to identify "this"
	// provisioner's PVs.
	identity string
}

func newSnapshotProvisioner(client kubernetes.Interface, crdclient *rest.RESTClient, id string) controller.Provisioner {
	return &snapshotProvisioner{
		client:    client,
		crdclient: crdclient,
		identity:  id,
	}
}

var _ controller.Provisioner = &snapshotProvisioner{}

func (p *snapshotProvisioner) snapshotRestore(snapshotName string, snapshotData crdv1.VolumeSnapshotData, options controller.ProvisionOptions) (*v1.PersistentVolumeSource, map[string]string, error) {
	// validate the PV supports snapshot and restore
	spec := &snapshotData.Spec
	volumeType := crdv1.GetSupportedVolumeFromSnapshotDataSpec(spec)
	if len(volumeType) == 0 {
		return nil, nil, fmt.Errorf("unsupported volume type found in SnapshotData %#v", *spec)
	}
	plugin, ok := volumePlugins[volumeType]
	if !ok {
		return nil, nil, fmt.Errorf("%s is not supported volume for %#v", volumeType, *spec)
	}

	// restore snapshot
	pvSrc, labels, err := plugin.SnapshotRestore(&snapshotData, options.PVC, options.PVName, options.StorageClass.Parameters)
	if err != nil {
		glog.Warningf("failed to snapshot %#v, err: %v", spec, err)
	} else {
		glog.Infof("snapshot %#v to snap %#v", spec, pvSrc)
	}

	return pvSrc, labels, err
}

// Provision creates a storage asset and returns a PV object representing it.
func (p *snapshotProvisioner) Provision(ctx context.Context, options controller.ProvisionOptions) (*v1.PersistentVolume, controller.ProvisioningState, error) {
	if options.PVC.Spec.Selector != nil {
		return nil, controller.ProvisioningFinished, fmt.Errorf("claim Selector is not supported")
	}
	snapshotName, ok := options.PVC.Annotations[crdclient.SnapshotPVCAnnotation]
	if !ok {
		return nil, controller.ProvisioningFinished, fmt.Errorf("snapshot annotation not found on PVC")
	}

	var snapshot crdv1.VolumeSnapshot
	err := p.crdclient.Get().
		Resource(crdv1.VolumeSnapshotResourcePlural).
		Namespace(options.PVC.Namespace).
		Name(snapshotName).
		Do(context.TODO()).Into(&snapshot)

	if err != nil {
		return nil, controller.ProvisioningInBackground, fmt.Errorf("failed to retrieve VolumeSnapshot %s in namespace %s: %v", snapshotName, options.PVC.Namespace, err)
	}
	// FIXME: should also check if any VolumeSnapshotData points to this VolumeSnapshot
	if len(snapshot.Spec.SnapshotDataName) == 0 {
		return nil, controller.ProvisioningNoChange, fmt.Errorf("VolumeSnapshot %s is not bound to any VolumeSnapshotData", snapshotName)
	}
	var snapshotData crdv1.VolumeSnapshotData
	err = p.crdclient.Get().
		Resource(crdv1.VolumeSnapshotDataResourcePlural).
		Name(snapshot.Spec.SnapshotDataName).
		Do(context.TODO()).Into(&snapshotData)

	if err != nil {
		return nil, controller.ProvisioningInBackground, fmt.Errorf("failed to retrieve VolumeSnapshotData %s: %v", snapshot.Spec.SnapshotDataName, err)
	}
	glog.V(3).Infof("restore from VolumeSnapshotData %s", snapshot.Spec.SnapshotDataName)

	pvSrc, labels, err := p.snapshotRestore(snapshot.Spec.SnapshotDataName, snapshotData, options)
	if err != nil || pvSrc == nil {
		return nil, controller.ProvisioningInBackground, fmt.Errorf("failed to create a PV from snapshot %s: %v", snapshotName, err)
	}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
			Annotations: map[string]string{
				provisionerIDAnn: p.identity,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: *options.StorageClass.DeepCopy().ReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: *pvSrc,
		},
	}

	if len(labels) != 0 {
		if pv.Labels == nil {
			pv.Labels = make(map[string]string)
		}
		for k, v := range labels {
			pv.Labels[k] = v
		}
	}

	glog.Infof("successfully created Snapshot share %#v", pv)

	return pv, controller.ProvisioningFinished, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *snapshotProvisioner) Delete(ctx context.Context, volume *v1.PersistentVolume) error {
	ann, ok := volume.Annotations[provisionerIDAnn]
	if !ok {
		return errors.New("identity annotation not found on PV")
	}
	if ann != p.identity {
		return &controller.IgnoredError{Reason: "identity annotation on PV does not match ours"}
	}

	volumeType := crdv1.GetSupportedVolumeFromPVSpec(&volume.Spec)
	if len(volumeType) == 0 {
		return fmt.Errorf("unsupported volume type found in PV %#v", *volume)
	}
	plugin, ok := volumePlugins[volumeType]
	if !ok {
		return fmt.Errorf("%s is not supported volume for %#v", volumeType, *volume)
	}

	// delete PV
	return plugin.VolumeDelete(volume)
}

var (
	master          = flag.String("master", "", "Master URL")
	kubeconfig      = flag.String("kubeconfig", "", "Absolute path to the kubeconfig")
	id              = flag.String("id", "", "Unique provisioner identity")
	cloudProvider   = flag.String("cloudprovider", "", "")
	cloudConfigFile = flag.String("cloudconfig", "", "Path to a Cloud config. Only required if cloudprovider is set.")
	volumePlugins   = make(map[string]volume.Plugin)
)

func main() {
	flag.Parse()
	flag.Set("logtostderr", "true")

	var config *rest.Config
	var err error
	if *master != "" || *kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags(*master, *kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	prID := provisionerName
	if *id != "" {
		prID = *id
	}
	if err != nil {
		glog.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create client: %v", err)
	}

	// build volume plugins map
	buildVolumePlugins()

	// make a crd client to list VolumeSnapshot
	snapshotClient, _, err := crdclient.NewClient(config)
	if err != nil || snapshotClient == nil {
		glog.Fatalf("Failed to make CRD client: %v", err)
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	snapshotProvisioner := newSnapshotProvisioner(clientset, snapshotClient, prID)

	// Start the provision controller which will dynamically provision snapshot
	// PVs
	pc := controller.NewProvisionController(
		clientset,
		provisionerName,
		snapshotProvisioner,
		controller.LeaderElection(isLeaderElectionEnabled()),
	)
	glog.Infof("starting PV provisioner %s", provisionerName)
	pc.Run(context.Background())
}

func buildVolumePlugins() {
	volumePlugins[gluster.GetPluginName()] = gluster.RegisterPlugin()
	volumePlugins[hostpath.GetPluginName()] = hostpath.RegisterPlugin()
	volumePlugins[openebs.GetPluginName()] = openebs.RegisterPlugin()

}

// isLeaderElectionEnabled returns true/false based on the ENV
// LEADER_ELECTION_ENABLED set via snaphot provisioner deployment.
// Defaults to true, means leaderElection enabled by default.
func isLeaderElectionEnabled() bool {
	leaderElection := os.Getenv(LeaderElectionKey)

	var leader bool
	switch strings.ToLower(leaderElection) {
	default:
		glog.Info("Leader election enabled for snapshot-provisioner")
		leader = true
	case "y", "yes", "true":
		glog.Info("Leader election enabled for snapshot-provisioner via leaderElectionKey")
		leader = true
	case "n", "no", "false":
		glog.Info("Leader election disabled for snapshot-provisioner via leaderElectionKey")
		leader = false
	}
	return leader
}
