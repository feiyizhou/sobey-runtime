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
	Resource   Resource   `json:"resource"`
}

type Resource struct {
	CpuPeriod              int64             `json:"cpuPeriod"`
	CpuQuota               int64             `json:"cpuQuota"`
	CpuShares              int64             `json:"cpuShares"`
	MemoryLimitInBytes     int64             `json:"memoryLimitInBytes"`
	OomScoreAdj            int64             `json:"oomScoreAdj"`
	CpusetCpus             string            `json:"cpusetCpus"`
	CpusetMems             string            `json:"cpusetMems"`
	HugepageLimits         []HugepageLimit   `json:"hugepageLimits"`
	Unified                map[string]string `json:"unified"`
	MemorySwapLimitInBytes int64             `json:"memorySwapLimitInBytes"`
}

type HugepageLimit struct {
	PageSize string `json:"pageSize,omitempty"`
	Limit    uint64 `json:"limit,omitempty"`
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
