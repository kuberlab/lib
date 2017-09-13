package utils

import (
	"fmt"
	"os"
	"sort"
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

func RankByWordCount(wordFrequencies map[string]int) PairList {
	pl := make(PairList, len(wordFrequencies))
	i := 0
	for k, v := range wordFrequencies {
		pl[i] = Pair{k, v}
		i++
	}
	sort.Sort(sort.Reverse(pl))
	return pl
}

type Pair struct {
	Key   string
	Value int
}

type PairList []Pair

func (p PairList) Len() int           { return len(p) }
func (p PairList) Less(i, j int) bool { return p[i].Value < p[j].Value }
func (p PairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
