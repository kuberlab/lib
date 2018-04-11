package utils

import (
	"os"
)

const (
	Namespace              = "POD_NAMESPACE"
	DefaultNamespace       = "kuberlab"
	Name                   = "POD_NAME"
	LogLevel               = "LOG_LEVEL"
	MasterJob              = "MASTER_JOB"
	DefaultGPUNodeSelector = "DEFAULT_GPU_NODE_SELECTOR"
	DefaultCPUNodeSelector = "DEFAULT_CPU_NODE_SELECTOR"
)

func getFromEnv(varName string) string {
	return os.Getenv(varName)
}

func GetLogLevel() string {
	return getFromEnv(LogLevel)
}

func GetNamespace() string {
	namespace := getFromEnv(Namespace)
	if namespace == "" {
		return DefaultNamespace
	}
	return namespace
}

func GetName() string {
	return getFromEnv(Name)
}

func GetMasterJob() string {
	return getFromEnv(MasterJob)
}

func GetDefaultGPUNodeSelector() string {
	return getFromEnv(DefaultGPUNodeSelector)
}

func GetDefaultCPUNodeSelector() string {
	return getFromEnv(DefaultCPUNodeSelector)
}
