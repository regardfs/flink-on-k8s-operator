/*
Copyright 2019 Google LLC.

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
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"math"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Validator validates CUD requests for the CR.
type Validator struct{}

// ValidateCreate validates create request.
func (v *Validator) ValidateCreate(cluster *FlinkCluster) error {
	var err error
	err = v.validateMeta(&cluster.ObjectMeta)
	if err != nil {
		return err
	}
	err = v.validateImage(&cluster.Spec.Image)
	if err != nil {
		return err
	}
	err = v.validateJobManager(&cluster.Spec.JobManager)
	if err != nil {
		return err
	}
	err = v.validateTaskManager(&cluster.Spec.TaskManager)
	if err != nil {
		return err
	}
	err = v.validateJob(cluster.Spec.Job)
	if err != nil {
		return err
	}
	return nil
}

// ValidateUpdate validates update request.
func (v *Validator) ValidateUpdate(old *FlinkCluster, new *FlinkCluster) error {
	var cancelRequested, err = v.checkCancelRequested(old, new)
	if err != nil {
		return err
	}
	if cancelRequested {
		return nil
	}

	if !reflect.DeepEqual(new.Spec, old.Spec) {
		return fmt.Errorf("the cluster properties are not updatable")
	}

	return nil
}

func (v *Validator) checkCancelRequested(
	old *FlinkCluster, new *FlinkCluster) (bool, error) {
	if old.Spec.Job == nil || new.Spec.Job == nil {
		return false, nil
	}
	var restartJob = (old.Spec.Job.CancelRequested != nil && *old.Spec.Job.CancelRequested) &&
		(new.Spec.Job.CancelRequested == nil || !*new.Spec.Job.CancelRequested)
	if restartJob {
		return false, fmt.Errorf(
			"updating cancelRequested from true to false is not allowed")
	}

	var stopJob = (old.Spec.Job.CancelRequested == nil || !*old.Spec.Job.CancelRequested) &&
		(new.Spec.Job.CancelRequested != nil && *new.Spec.Job.CancelRequested)
	if stopJob {
		// Check if only `cancelRequested` changed, no other changes.
		var oldCopy = old.DeepCopy()
		oldCopy.Spec.Job.CancelRequested = new.Spec.Job.CancelRequested
		return reflect.DeepEqual(new.Spec, oldCopy.Spec), nil
	}

	return false, nil
}

func (v *Validator) validateMeta(meta *metav1.ObjectMeta) error {
	if len(meta.Name) == 0 {
		return fmt.Errorf("cluster name is unspecified")
	}
	if len(meta.Namespace) == 0 {
		return fmt.Errorf("cluster namesapce is unspecified")
	}
	return nil
}

func (v *Validator) validateImage(imageSpec *ImageSpec) error {
	if len(imageSpec.Name) == 0 {
		return fmt.Errorf("image name is unspecified")
	}
	switch imageSpec.PullPolicy {
	case corev1.PullAlways:
	case corev1.PullIfNotPresent:
	case corev1.PullNever:
	default:
		return fmt.Errorf("invalid image pullPolicy: %v", imageSpec.PullPolicy)
	}
	return nil
}

func (v *Validator) validateJobManager(jmSpec *JobManagerSpec) error {
	var err error

	// Replicas.
	if jmSpec.Replicas == nil || *jmSpec.Replicas != 1 {
		return fmt.Errorf("invalid JobManager replicas, it must be 1")
	}

	// AccessScope.
	switch jmSpec.AccessScope {
	case AccessScope.Cluster:
	case AccessScope.VPC:
	case AccessScope.External:
	default:
		return fmt.Errorf("invalid JobManager access scope: %v", jmSpec.AccessScope)
	}

	// Ports.
	err = v.validatePort(jmSpec.Ports.RPC, "rpc", "jobmanager")
	if err != nil {
		return err
	}
	err = v.validatePort(jmSpec.Ports.Blob, "blob", "jobmanager")
	if err != nil {
		return err
	}
	err = v.validatePort(jmSpec.Ports.Query, "query", "jobmanager")
	if err != nil {
		return err
	}
	err = v.validatePort(jmSpec.Ports.UI, "ui", "jobmanager")
	if err != nil {
		return err
	}

	// MemoryOffHeapRatio
	if jmSpec.MemoryOffHeapRatio == nil || *jmSpec.MemoryOffHeapRatio > 100 || *jmSpec.MemoryOffHeapRatio < 0 {
		return fmt.Errorf("invalid JobManager memoryOffHeapRatio, it must be between 0 and 100")
	}

	// MemoryOffHeapMin
	divisor := resource.MustParse("1Mi")
	jmMemLimit := math.Floor(float64(jmSpec.Resources.Limits.Memory().Value()) / float64(divisor.Value()))
	if jmSpec.MemoryOffHeapMin != nil {
		return fmt.Errorf("invalid JobManager memory configuration, MemoryOffHeapMin is not specified")
	} else if *jmSpec.MemoryOffHeapMin > int32(jmMemLimit) {
		return fmt.Errorf("invalid JobManager memory configuration, memory limit must be larger than MemoryOffHeapMin")
	}

	return nil
}

func (v *Validator) validateTaskManager(tmSpec *TaskManagerSpec) error {
	// Replicas.
	if tmSpec.Replicas < 1 {
		return fmt.Errorf("invalid TaskManager replicas, it must >= 1")
	}

	// Ports.
	var err error
	err = v.validatePort(tmSpec.Ports.RPC, "rpc", "taskmanager")
	if err != nil {
		return err
	}
	err = v.validatePort(tmSpec.Ports.Data, "data", "taskmanager")
	if err != nil {
		return err
	}
	err = v.validatePort(tmSpec.Ports.Query, "query", "taskmanager")
	if err != nil {
		return err
	}

	return nil
}

func (v *Validator) validateJob(jobSpec *JobSpec) error {
	if jobSpec == nil {
		return nil
	}

	if len(jobSpec.JarFile) == 0 {
		return fmt.Errorf("job jarFile is unspecified")
	}

	if jobSpec.Parallelism == nil {
		return fmt.Errorf("job parallelism is unspecified")
	}
	if *jobSpec.Parallelism < 1 {
		return fmt.Errorf("job parallelism must be >= 1")
	}

	if jobSpec.RestartPolicy == nil {
		return fmt.Errorf("job restartPolicy is unspecified")
	}
	switch *jobSpec.RestartPolicy {
	case corev1.RestartPolicyNever:
	case corev1.RestartPolicyOnFailure:
	default:
		return fmt.Errorf("invalid job restartPolicy: %v", *jobSpec.RestartPolicy)
	}

	if jobSpec.CleanupPolicy == nil {
		return fmt.Errorf("job cleanupPolicy is unspecified")
	}
	var err = v.validateCleanupAction(
		"cleanupPolicy.afterJobSucceeds", jobSpec.CleanupPolicy.AfterJobSucceeds)
	if err != nil {
		return err
	}
	err = v.validateCleanupAction(
		"cleanupPolicy.afterJobFails", jobSpec.CleanupPolicy.AfterJobFails)
	if err != nil {
		return err
	}

	if jobSpec.CancelRequested != nil && *jobSpec.CancelRequested {
		return fmt.Errorf(
			"property `cancelRequested` cannot be set to true for a new job")
	}

	return nil
}

func (v *Validator) validatePort(
	port *int32, name string, component string) error {
	if port == nil {
		return fmt.Errorf("%v %v port is unspecified", component, name)
	}
	if *port <= 1024 {
		return fmt.Errorf(
			"invalid %v %v port: %v, must be > 1024", component, name, *port)
	}
	return nil
}

func (v *Validator) validateCleanupAction(
	property string, value CleanupAction) error {
	switch value {
	case CleanupActionDeleteCluster:
	case CleanupActionDeleteTaskManager:
	case CleanupActionKeepCluster:
	default:
		return fmt.Errorf(
			"invalid %v: %v",
			property, value)
	}
	return nil
}
