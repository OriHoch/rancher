package workload

import (
	"strconv"
	"strings"

	"fmt"

	"encoding/json"

	"github.com/rancher/types/apis/apps/v1beta2"
	batchv1 "github.com/rancher/types/apis/batch/v1"
	"github.com/rancher/types/apis/batch/v1beta1"
	"github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/config"
	corev1beta2 "k8s.io/api/apps/v1beta2"
	corebatchv1 "k8s.io/api/batch/v1"
	corebatchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	AppVersion                = "apps/v1beta2"
	BatchBetaVersion          = "batch/v1beta1"
	BatchVersion              = "batch/v1"
	WorkloadAnnotation        = "field.cattle.io/targetWorkloadIds"
	PortsAnnotation           = "field.cattle.io/ports"
	ClusterIPServiceType      = "ClusterIP"
	WorkloadLabel             = "workload.user.cattle.io/workload"
	AllWorkloads              = "_all_workloads_"
	DeploymentType            = "deployment"
	ReplicationControllerType = "replicationcontroller"
	ReplicaSetType            = "replicaset"
	DaemonSetType             = "daemonset"
	StatefulSetType           = "statefulset"
	JobType                   = "job"
	CronJobType               = "cronJob"
)

var WorkloadKinds = map[string]bool{
	"Deployment":            true,
	"ReplicationController": true,
	"ReplicaSet":            true,
	"DaemonSet":             true,
	"StatefulSet":           true,
	"Job":                   true,
	"CronJob":               true,
}

type Workload struct {
	Name            string
	Namespace       string
	UUID            types.UID
	SelectorLabels  map[string]string
	Annotations     map[string]string
	TemplateSpec    *corev1.PodTemplateSpec
	Kind            string
	APIVersion      string
	OwnerReferences []metav1.OwnerReference
	Labels          map[string]string
	Key             string
}

type CommonController struct {
	DeploymentLister            v1beta2.DeploymentLister
	ReplicationControllerLister v1.ReplicationControllerLister
	ReplicaSetLister            v1beta2.ReplicaSetLister
	DaemonSetLister             v1beta2.DaemonSetLister
	StatefulSetLister           v1beta2.StatefulSetLister
	JobLister                   batchv1.JobLister
	CronJobLister               v1beta1.CronJobLister
	Deployments                 v1beta2.DeploymentInterface
	ReplicationControllers      v1.ReplicationControllerInterface
	ReplicaSes                  v1beta2.ReplicaSetInterface
	DaemonSets                  v1beta2.DaemonSetInterface
	StatefulSets                v1beta2.StatefulSetInterface
	Jobs                        batchv1.JobInterface
	CronJobs                    v1beta1.CronJobInterface
	Sync                        func(key string, w *Workload) error
}

func NewWorkloadController(workload *config.UserOnlyContext, f func(key string, w *Workload) error) CommonController {
	c := CommonController{
		DeploymentLister:            workload.Apps.Deployments("").Controller().Lister(),
		ReplicationControllerLister: workload.Core.ReplicationControllers("").Controller().Lister(),
		ReplicaSetLister:            workload.Apps.ReplicaSets("").Controller().Lister(),
		DaemonSetLister:             workload.Apps.DaemonSets("").Controller().Lister(),
		StatefulSetLister:           workload.Apps.StatefulSets("").Controller().Lister(),
		JobLister:                   workload.BatchV1.Jobs("").Controller().Lister(),
		CronJobLister:               workload.BatchV1Beta1.CronJobs("").Controller().Lister(),
		Deployments:                 workload.Apps.Deployments(""),
		ReplicationControllers:      workload.Core.ReplicationControllers(""),
		ReplicaSes:                  workload.Apps.ReplicaSets(""),
		DaemonSets:                  workload.Apps.DaemonSets(""),
		StatefulSets:                workload.Apps.StatefulSets(""),
		Jobs:                        workload.BatchV1.Jobs(""),
		CronJobs:                    workload.BatchV1Beta1.CronJobs(""),
		Sync:                        f,
	}
	if f != nil {
		workload.Apps.Deployments("").AddHandler(getName(), c.syncDeployments)
		workload.Core.ReplicationControllers("").AddHandler(getName(), c.syncReplicationControllers)
		workload.Apps.ReplicaSets("").AddHandler(getName(), c.syncReplicaSet)
		workload.Apps.DaemonSets("").AddHandler(getName(), c.syncDaemonSet)
		workload.Apps.StatefulSets("").AddHandler(getName(), c.syncStatefulSet)
		workload.BatchV1.Jobs("").AddHandler(getName(), c.syncJob)
		workload.BatchV1Beta1.CronJobs("").AddHandler(getName(), c.syncCronJob)
	}
	return c
}

func (c *CommonController) syncDeployments(key string, obj *corev1beta2.Deployment) error {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil
	}
	var w *Workload
	var err error
	if key != AllWorkloads {
		w, err = c.getWorkload(key, DeploymentType)
		if err != nil || w == nil {
			return err
		}
	}

	return c.Sync(key, w)
}

func (c *CommonController) syncReplicationControllers(key string, obj *corev1.ReplicationController) error {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil
	}
	w, err := c.getWorkload(key, ReplicationControllerType)
	if err != nil || w == nil {
		return err
	}
	return c.Sync(key, w)
}

func (c *CommonController) syncReplicaSet(key string, obj *corev1beta2.ReplicaSet) error {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil
	}
	w, err := c.getWorkload(key, ReplicaSetType)
	if err != nil || w == nil {
		return err
	}
	return c.Sync(key, w)
}

func (c *CommonController) syncDaemonSet(key string, obj *corev1beta2.DaemonSet) error {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil
	}
	w, err := c.getWorkload(key, DaemonSetType)
	if err != nil || w == nil {
		return err
	}
	return c.Sync(key, w)
}

func (c *CommonController) syncStatefulSet(key string, obj *corev1beta2.StatefulSet) error {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil
	}
	w, err := c.getWorkload(key, StatefulSetType)
	if err != nil || w == nil {
		return err
	}
	return c.Sync(key, w)
}

func (c *CommonController) syncJob(key string, obj *corebatchv1.Job) error {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil
	}
	w, err := c.getWorkload(key, JobType)
	if err != nil || w == nil {
		return err
	}
	return c.Sync(key, w)
}

func (c *CommonController) syncCronJob(key string, obj *corebatchv1beta1.CronJob) error {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil
	}
	w, err := c.getWorkload(key, CronJobType)
	if err != nil || w == nil {
		return err
	}
	return c.Sync(key, w)
}

func (c CommonController) getWorkload(key string, objectType string) (*Workload, error) {
	splitted := strings.Split(key, "/")
	namespace := splitted[0]
	name := splitted[1]
	return c.GetByWorkloadID(getWorkloadID(objectType, namespace, name))
}

func (c CommonController) GetByWorkloadID(key string) (*Workload, error) {
	splitted := strings.Split(key, ":")
	if len(splitted) != 3 {
		return nil, fmt.Errorf("workload name [%s] is invalid", key)
	}
	workloadType := strings.ToLower(splitted[0])
	namespace := splitted[1]
	name := splitted[2]
	var workload *Workload
	switch workloadType {
	case ReplicationControllerType:
		o, err := c.ReplicationControllerLister.Get(namespace, name)
		if err != nil || o.DeletionTimestamp != nil {
			return nil, err
		}
		labelSelector := &metav1.LabelSelector{
			MatchLabels: o.Spec.Selector,
		}
		workload = getWorkload(namespace, name, workloadType, AppVersion, o.UID, labelSelector, o.Annotations, o.Spec.Template, o.OwnerReferences, o.Labels)
	case ReplicaSetType:
		o, err := c.ReplicaSetLister.Get(namespace, name)
		if err != nil || o.DeletionTimestamp != nil {
			return nil, err
		}

		workload = getWorkload(namespace, name, workloadType, AppVersion, o.UID, o.Spec.Selector, o.Annotations, &o.Spec.Template, o.OwnerReferences, o.Labels)
	case DaemonSetType:
		o, err := c.DaemonSetLister.Get(namespace, name)
		if err != nil || o.DeletionTimestamp != nil {
			return nil, err
		}

		workload = getWorkload(namespace, name, workloadType, AppVersion, o.UID, o.Spec.Selector, o.Annotations, &o.Spec.Template, o.OwnerReferences, o.Labels)
	case StatefulSetType:
		o, err := c.StatefulSetLister.Get(namespace, name)
		if err != nil || o.DeletionTimestamp != nil {
			return nil, err
		}

		workload = getWorkload(namespace, name, workloadType, AppVersion, o.UID, o.Spec.Selector, o.Annotations, &o.Spec.Template, o.OwnerReferences, o.Labels)
	case JobType:
		o, err := c.JobLister.Get(namespace, name)
		if err != nil || o.DeletionTimestamp != nil {
			return nil, err
		}
		var labelSelector *metav1.LabelSelector
		if o.Spec.Selector != nil {
			labelSelector = &metav1.LabelSelector{
				MatchLabels: o.Spec.Selector.MatchLabels,
			}
		}

		workload = getWorkload(namespace, name, workloadType, BatchVersion, o.UID, labelSelector, o.Annotations, &o.Spec.Template, o.OwnerReferences, o.Labels)
	case CronJobType:
		o, err := c.CronJobLister.Get(namespace, name)
		if err != nil || o.DeletionTimestamp != nil {
			return nil, err
		}
		var labelSelector *metav1.LabelSelector
		if o.Spec.JobTemplate.Spec.Selector != nil {
			labelSelector = &metav1.LabelSelector{
				MatchLabels: o.Spec.JobTemplate.Spec.Selector.MatchLabels,
			}
		}

		workload = getWorkload(namespace, name, workloadType, BatchBetaVersion, o.UID, labelSelector, o.Annotations, &o.Spec.JobTemplate.Spec.Template, o.OwnerReferences, o.Labels)
	default:
		o, err := c.DeploymentLister.Get(namespace, name)
		if err != nil || o.DeletionTimestamp != nil {
			return nil, err
		}

		workload = getWorkload(namespace, name, DeploymentType, AppVersion, o.UID, o.Spec.Selector, o.Annotations, &o.Spec.Template, o.OwnerReferences, o.Labels)
	}
	return workload, nil
}

func getWorkload(namespace string, name string, kind string, apiVersion string, UUID types.UID, selectorLabels *metav1.LabelSelector,
	annotations map[string]string, podTemplateSpec *corev1.PodTemplateSpec, ownerRefs []metav1.OwnerReference, labels map[string]string) *Workload {
	return &Workload{
		Name:            name,
		Namespace:       namespace,
		SelectorLabels:  getSelectorLables(selectorLabels),
		UUID:            UUID,
		Annotations:     annotations,
		TemplateSpec:    podTemplateSpec,
		OwnerReferences: ownerRefs,
		Kind:            kind,
		APIVersion:      apiVersion,
		Labels:          labels,
		Key:             fmt.Sprintf("%s/%s", namespace, name),
	}
}

func (c CommonController) GetAllWorkloads(namespace string) ([]*Workload, error) {
	var workloads []*Workload

	// deployments
	ds, err := c.DeploymentLister.List(namespace, labels.NewSelector())
	if err != nil {
		return workloads, err
	}

	for _, o := range ds {
		workload, err := c.GetByWorkloadID(getWorkloadID(DeploymentType, o.Namespace, o.Name))
		if err != nil || workload == nil {
			return workloads, err
		}
		workloads = append(workloads, workload)
	}

	// replication controllers
	rcs, err := c.ReplicationControllerLister.List(namespace, labels.NewSelector())
	if err != nil {
		return workloads, err
	}

	for _, o := range rcs {
		workload, err := c.GetByWorkloadID(getWorkloadID(ReplicationControllerType, o.Namespace, o.Name))
		if err != nil || workload == nil {
			return workloads, err
		}
		workloads = append(workloads, workload)
	}

	// replica sets
	rss, err := c.ReplicaSetLister.List(namespace, labels.NewSelector())
	if err != nil {
		return workloads, err
	}

	for _, o := range rss {
		workload, err := c.GetByWorkloadID(getWorkloadID(ReplicaSetType, o.Namespace, o.Name))
		if err != nil || workload == nil {
			return workloads, err
		}
		workloads = append(workloads, workload)
	}

	// daemon sets
	dss, err := c.DaemonSetLister.List(namespace, labels.NewSelector())
	if err != nil {
		return workloads, err
	}

	for _, o := range dss {
		workload, err := c.GetByWorkloadID(getWorkloadID(DaemonSetType, o.Namespace, o.Name))
		if err != nil || workload == nil {
			return workloads, err
		}
		workloads = append(workloads, workload)
	}

	// stateful sets
	sts, err := c.StatefulSetLister.List(namespace, labels.NewSelector())
	if err != nil {
		return workloads, err
	}

	for _, o := range sts {
		workload, err := c.GetByWorkloadID(getWorkloadID(StatefulSetType, o.Namespace, o.Name))
		if err != nil || workload == nil {
			return workloads, err
		}
		workloads = append(workloads, workload)
	}

	// jobs
	jobs, err := c.JobLister.List(namespace, labels.NewSelector())
	if err != nil {
		return workloads, err
	}

	for _, o := range jobs {
		workload, err := c.GetByWorkloadID(getWorkloadID(JobType, o.Namespace, o.Name))
		if err != nil || workload == nil {
			return workloads, err
		}
		workloads = append(workloads, workload)
	}

	// cron jobs
	cronJobs, err := c.CronJobLister.List(namespace, labels.NewSelector())
	if err != nil {
		return workloads, err
	}

	for _, o := range cronJobs {
		workload, err := c.GetByWorkloadID(getWorkloadID(CronJobType, o.Namespace, o.Name))
		if err != nil || workload == nil {
			return workloads, err
		}
		workloads = append(workloads, workload)
	}

	return workloads, nil
}

func (c CommonController) GetWorkloadsMatchingLabels(namespace string, targetLabels map[string]string) ([]*Workload, error) {
	var workloads []*Workload
	allWorkloads, err := c.GetAllWorkloads(namespace)
	if err != nil {
		return workloads, err
	}

	for _, workload := range allWorkloads {
		workloadSelector := labels.SelectorFromSet(workload.SelectorLabels)
		if workloadSelector.Matches(labels.Set(targetLabels)) {
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func (c CommonController) GetWorkloadsMatchingSelector(namespace string, selectorLabels map[string]string) ([]*Workload, error) {
	var workloads []*Workload
	allWorkloads, err := c.GetAllWorkloads(namespace)
	if err != nil {
		return workloads, err
	}

	selector := labels.SelectorFromSet(selectorLabels)
	for _, workload := range allWorkloads {
		if selector.Matches(labels.Set(workload.Labels)) {
			workloads = append(workloads, workload)
		}
	}

	return workloads, nil
}

func getSelectorLables(s *metav1.LabelSelector) map[string]string {
	if s == nil {
		return nil
	}
	selectorLabels := map[string]string{}
	for key, value := range s.MatchLabels {
		selectorLabels[key] = value
	}
	return selectorLabels
}

type Service struct {
	Type         corev1.ServiceType
	ClusterIP    string
	ServicePorts []corev1.ServicePort
}

type ContainerPort struct {
	Kind          string `json:"kind,omitempty"`
	SourcePort    int    `json:"sourcePort,omitempty"`
	DNSName       string `json:"dnsName,omitempty"`
	Name          string `json:"name,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	ContainerPort int32  `json:"containerPort,omitempty"`
}

func generateServiceFromContainers(workload *Workload) *Service {
	var servicePorts []corev1.ServicePort
	for _, c := range workload.TemplateSpec.Spec.Containers {
		for _, p := range c.Ports {
			var portName string
			if p.Name == "" {
				portName = fmt.Sprintf("%s-%s", strconv.FormatInt(int64(p.ContainerPort), 10), c.Name)
			} else {
				portName = fmt.Sprintf("%s-%s", p.Name, c.Name)
			}
			servicePort := corev1.ServicePort{
				Port:       p.ContainerPort,
				TargetPort: intstr.Parse(strconv.FormatInt(int64(p.ContainerPort), 10)),
				Protocol:   p.Protocol,
				Name:       portName,
			}

			servicePorts = append(servicePorts, servicePort)
		}
	}

	return &Service{
		Type:         ClusterIPServiceType,
		ClusterIP:    "None",
		ServicePorts: servicePorts,
	}
}

func generateServicesFromPortsAnnotation(portAnnotation string) ([]Service, error) {
	var services []Service
	var ports []ContainerPort
	err := json.Unmarshal([]byte(portAnnotation), &ports)
	if err != nil {
		return services, err
	}

	svcTypeToPort := map[corev1.ServiceType][]ContainerPort{}
	for _, port := range ports {
		if port.Kind == "HostPort" {
			continue
		}
		svcType := corev1.ServiceType(port.Kind)
		svcTypeToPort[svcType] = append(svcTypeToPort[svcType], port)
	}

	for svcType, ports := range svcTypeToPort {
		var servicePorts []corev1.ServicePort
		for _, p := range ports {
			servicePort := corev1.ServicePort{
				Port:       p.ContainerPort,
				TargetPort: intstr.Parse(strconv.FormatInt(int64(p.ContainerPort), 10)),
				Protocol:   corev1.Protocol(p.Protocol),
				Name:       p.Name,
			}
			servicePorts = append(servicePorts, servicePort)
		}
		services = append(services, Service{
			Type:         svcType,
			ServicePorts: servicePorts,
		})
	}

	return services, nil
}

func (wk Workload) getKey() string {
	return fmt.Sprintf("%s:%s:%s", wk.Kind, wk.Namespace, wk.Name)
}

func getWorkloadID(objectType string, namespace string, name string) string {
	return fmt.Sprintf("%s:%s:%s", objectType, namespace, name)
}

func (c CommonController) UpdateWorkload(w *Workload) error {
	// only annotations updates are supported
	switch w.Kind {
	case DeploymentType:
		o, err := c.DeploymentLister.Get(w.Namespace, w.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		toUpdate := o.DeepCopy()
		toUpdate.Annotations = w.Annotations
		_, err = c.Deployments.Update(toUpdate)
		if err != nil {
			return err
		}
	case ReplicationControllerType:
		o, err := c.ReplicationControllerLister.Get(w.Namespace, w.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		toUpdate := o.DeepCopy()
		toUpdate.Annotations = w.Annotations
		_, err = c.ReplicationControllers.Update(toUpdate)
		if err != nil {
			return err
		}
	case ReplicaSetType:
		o, err := c.ReplicaSetLister.Get(w.Namespace, w.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		toUpdate := o.DeepCopy()
		toUpdate.Annotations = w.Annotations
		_, err = c.ReplicaSes.Update(toUpdate)
		if err != nil {
			return err
		}
	case DaemonSetType:
		o, err := c.DaemonSetLister.Get(w.Namespace, w.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		toUpdate := o.DeepCopy()
		toUpdate.Annotations = w.Annotations
		_, err = c.DaemonSets.Update(toUpdate)
		if err != nil {
			return err
		}
	case StatefulSetType:
		o, err := c.StatefulSetLister.Get(w.Namespace, w.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		toUpdate := o.DeepCopy()
		toUpdate.Annotations = w.Annotations
		_, err = c.StatefulSets.Update(toUpdate)
		if err != nil {
			return err
		}
	case JobType:
		o, err := c.JobLister.Get(w.Namespace, w.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		toUpdate := o.DeepCopy()
		toUpdate.Annotations = w.Annotations
		_, err = c.Jobs.Update(toUpdate)
		if err != nil {
			return err
		}
	case CronJobType:
		o, err := c.CronJobLister.Get(w.Namespace, w.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		toUpdate := o.DeepCopy()
		toUpdate.Annotations = w.Annotations
		_, err = c.CronJobs.Update(toUpdate)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c CommonController) EnqueueWorkload(w *Workload) {
	switch w.Kind {
	case DeploymentType:
		c.Deployments.Controller().Enqueue(w.Namespace, w.Name)
	case ReplicationControllerType:
		c.ReplicationControllers.Controller().Enqueue(w.Namespace, w.Name)
	case ReplicaSetType:
		c.ReplicaSes.Controller().Enqueue(w.Namespace, w.Name)
	case DaemonSetType:
		c.DaemonSets.Controller().Enqueue(w.Namespace, w.Name)
	case StatefulSetType:
		c.StatefulSets.Controller().Enqueue(w.Namespace, w.Name)
	case JobType:
		c.Jobs.Controller().Enqueue(w.Namespace, w.Name)
	case CronJobType:
		c.CronJobs.Controller().Enqueue(w.Namespace, w.Name)
	}
}

func (c CommonController) EnqueueAllWorkloads(namespace string) error {
	ws, err := c.GetAllWorkloads(namespace)
	if err != nil {
		return err
	}
	for _, w := range ws {
		c.EnqueueWorkload(w)
	}
	return nil
}
