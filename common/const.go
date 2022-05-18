package common

var InternalLabelKeys = []string{ContainerTypeLabelKey, ContainerLogPathLabelKey, SandboxIDLabelKey}

const (
	ContainerTypeLabelKey       = "io.kubernetes.sobey.type"
	ContainerTypeLabelSandbox   = "podsandbox"
	ContainerTypeLabelContainer = "container"
	ContainerLogPathLabelKey    = "io.kubernetes.container.logpath"
	SandboxIDLabelKey           = "io.kubernetes.sandbox.id"

	SobeyRuntimeApiVersion = "1.0.0"

	KubernetesPodNameLabel       = "io.kubernetes.pod.name"
	KubernetesPodNamespaceLabel  = "io.kubernetes.pod.namespace"
	KubernetesPodUIDLabel        = "io.kubernetes.pod.uid"
	KubernetesContainerNameLabel = "io.kubernetes.container.name"

	ServerLogDirPath        = "/var/lib/sobey/servers/log/"
	KubernetesPodLogDirPath = "/var/log/pods/"
)
