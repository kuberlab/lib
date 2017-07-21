package utils

import (
	"os"
)

const (
	Namespace = "POD_NAMESPACE"
	Name      = "POD_NAME"
	LogLevel  = "LOG_LEVEL"
	MasterJob = "MASTER_JOB"
)

func getFromEnv(varName string) string {
	return os.Getenv(varName)
}

func GetLogLevel() string {
	return getFromEnv(LogLevel)
}

func GetNamespace() string {
	return getFromEnv(Namespace)
}

func GetName() string {
	return getFromEnv(Name)
}

func GetMasterJob() string {
	return getFromEnv(MasterJob)
}
