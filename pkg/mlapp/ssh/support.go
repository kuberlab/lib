package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TaskSshSupport(name, namespace string, labels map[string]string) (*v1.Secret, []v1.Volume, []v1.VolumeMount) {
	name = name + "-ssh"
	publicKey, privateKey := makeSSHKeyPair()

	data := map[string]string{
		"id_rsa":          privateKey,
		"id_rsa.pub":      publicKey,
		"authorized_keys": publicKey,
	}
	ks := &v1.Secret{
		Type: v1.SecretType("Opaque"),
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		StringData: data,
	}
	var mode int32 = 0600
	items := []v1.KeyToPath{
		{
			Key:  "id_rsa",
			Path: "/root/.ssh/id_rsa",
			Mode: &mode,
		},
		{
			Key:  "id_rsa.pub",
			Path: "/root/.ssh/id_rsa.pub",
			Mode: &mode,
		},
		{
			Key:  "authorized_keys",
			Path: "/root/.ssh/authorized_keys",
			Mode: &mode,
		},
	}
	sshAccessVolume := v1.Volume{
		Name: "ssh-task-access",
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: name,
				Items:      items,
			},
		},
	}
	sshAccessVolumeMount := []v1.VolumeMount{
		{
			Name:      "ssh-task-access",
			MountPath: "/root/.ssh/authorized_keys",
			SubPath:   "authorized_keys",
		},
		{
			Name:      "ssh-task-access",
			MountPath: "/root/.ssh/id_rsa.pub",
			SubPath:   "id_rsa.pub",
		},
		{
			Name:      "ssh-task-access",
			MountPath: "/root/.ssh/id_rsa",
			SubPath:   "id_rsa",
		},
	}
	return ks, []v1.Volume{sshAccessVolume}, sshAccessVolumeMount
}

func makeSSHKeyPair() (string, string) {
	prKey, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		return "", ""
	}

	if err != nil {
		return "", ""
	}
	privateKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(prKey)}
	private := pem.EncodeToMemory(privateKeyPEM)

	// generate and write public key
	pub, err := ssh.NewPublicKey(&prKey.PublicKey)
	if err != nil {
		return "", ""
	}
	public := ssh.MarshalAuthorizedKey(pub)

	return string(public), string(private)
}
