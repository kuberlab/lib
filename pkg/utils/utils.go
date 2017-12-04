package utils

import (
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
)

var charNotFitToKube = regexp.MustCompile("[^-a-z0-9]")
var charNotFitToLabel = regexp.MustCompile("[^-a-zA-Z0-9_]")
var charNotFitToEnv = regexp.MustCompile("[^_A-Z0-9]")

func IntPtr(i int) *int {
	return &i
}
func StrPtr(s string) *string {
	return &s
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

func JoinMaps(dest map[string]string, srcs ...map[string]string) map[string]string {
	for _, src := range srcs {
		for k, v := range src {
			dest[k] = v
		}
	}
	return dest
}

func MustParse(date string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", date, time.FixedZone("UTC", 0))
	if err != nil {
		panic(err)
	}
	return t
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

func EnvConvert(v string) string {
	res := strings.ToUpper(v)
	res = strings.Replace(res, "-", "_", -1)
	res = charNotFitToEnv.ReplaceAllString(res, "_")
	return res
}

func hash(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	//return h.Sum32()
	return strconv.FormatUint(uint64(h.Sum32()), 16)
}

func KubeEncode(v string, lower bool, regexp *regexp.Regexp, lengthLimit int) string {
	res := v
	if lower {
		res = strings.ToLower(res)
	}
	res = regexp.ReplaceAllString(res, "-")

	h := hash(v)
	hlen := len(h) + 1

	if len(res) < lengthLimit {
		return res
	} else {
		edge := lengthLimit - hlen
		return res[:edge] + "-" + h
	}
}

func KubeNamespaceEncode(v string) string {
	return KubeEncode(v, true, charNotFitToKube, 63)
}

func KubeDeploymentEncode(v string) string {
	return KubeEncode(v, true, charNotFitToKube, 63)
}

func KubePodNameEncode(v string) string {
	return KubeEncode(v, true, charNotFitToKube, 253)
}

func KubeLabelEncode(v string) string {
	return KubeEncode(v, false, charNotFitToLabel, 63)
}
