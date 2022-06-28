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
	ServerImageDirPath      = "/var/lib/sobey/images/"
	KubernetesPodLogDirPath = "/var/log/pods/"

	SandboxIDPrefix   = "sandbox"
	ContainerIDPrefix = "container"

	PauseShellPath  = "/root/pause.sh"
	SecretShellPath = "/root/secret.sh"
	NginxShellPath  = "/root/nginx.sh"
)
