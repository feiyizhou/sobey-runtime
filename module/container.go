package module

type ContainerConf struct {
	ID         string     `json:"id"`
	SandboxPid string     `json:"sandboxPid"`
	Mem        int64      `json:"mem"`
	Swap       int64      `json:"swap"`
	PIDs       int        `json:"pids"`
	CPUs       float64    `json:"cpus"`
	Image      Image      `json:"image"`
	Args       []string   `json:"args"`
	Env        []KeyValue `json:"env"`
	Mount      []Mount    `json:"mount"`
}

type Image struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Mount struct {
	ContainerPath string `json:"containerPath"`
	HostPath      string `json:"hostPath"`
}
