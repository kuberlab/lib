package utils

import (
	"os"

	"fmt"
	"strings"

	"github.com/Sirupsen/logrus"
)

func IntPtr(i int) *int {
	return &i
}

func LogExit(status int) {
	logrus.Infof("Exiting with status: %v", status)
	os.Exit(status)
}

func GetCallback() (string, error) {
	if v := os.Getenv("USE_CALLBACK_ADDR"); v != "" {
		return v, nil
	}
	ip, err := ChooseHostInterface()
	if err != nil {
		return "", err
	}
	hostname := fmt.Sprintf("http://%v.%v.pod.cluster.local", strings.Replace(ip.String(), ".", "-", -1), GetNamespace())
	return hostname, nil
}

func JoinMaps(dest map[string]string, srcs ...map[string]string) {
	for _, src := range srcs {
		for k, v := range src {
			dest[k] = v
		}
	}
}
