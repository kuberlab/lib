package utils

import (
	"os"

	"github.com/Sirupsen/logrus"
)

func IntPtr(i int) *int {
	return &i
}

func LogExit(status int) {
	logrus.Infof("Exiting with status: %v", status)
	os.Exit(status)
}
