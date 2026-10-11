// Purpose: the kubectl implementation of Exec and the postgres pod discovery.
//
// Inputs: --cluster (kubeconfig path), --namespace, the Helm release name.
//
// Outputs: an Exec whose commands run as
// `kubectl exec -i [--kubeconfig K] [-n NS] <pod> -c postgres -- <argv>`.
//
// Constraints: kubectl comes from PATH (E730 when missing). The pod is found by
// the chart's labels app.kubernetes.io/name=postgres and
// app.kubernetes.io/instance=<release>: exactly one pod must be Running with the
// postgres container ready, else E731 says how many were found. Nothing secret
// is on the kubectl argv.
package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// postgresContainer is the container name the chart gives the postgres service.
const postgresContainer = "postgres"

// lookPath is replaced by tests.
var lookPath = exec.LookPath

// NewKubectl finds kubectl and the one running postgres pod of release.
func NewKubectl(ctx context.Context, kubeconfig, namespace, release string) (Exec, error) {
	bin, err := lookPath("kubectl")
	if err != nil {
		return nil, errKubectlMissing()
	}
	env := cleanEnv(os.Environ())
	list := &cmdExec{bin: bin, env: env}
	var out, serr bytes.Buffer
	if err := list.Run(ctx, getPodsArgv(kubeconfig, namespace, release), nil, &out, &serr); err != nil {
		return nil, errPod("the postgres pod could not be looked up", exitCause(err))
	}
	pod, err := pickPod(out.Bytes())
	if err != nil {
		return nil, err
	}
	return &cmdExec{bin: bin, prefix: execPrefix(kubeconfig, namespace, pod), env: env}, nil
}

func kubeFlags(kubeconfig, namespace string) []string {
	var a []string
	if kubeconfig != "" {
		a = append(a, "--kubeconfig", kubeconfig)
	}
	if namespace != "" {
		a = append(a, "-n", namespace)
	}
	return a
}

func getPodsArgv(kubeconfig, namespace, release string) []string {
	a := append([]string{"get", "pods"}, kubeFlags(kubeconfig, namespace)...)
	return append(a, "-l", "app.kubernetes.io/name=postgres,app.kubernetes.io/instance="+release, "-o", "json")
}

func execPrefix(kubeconfig, namespace, pod string) []string {
	a := append([]string{"exec", "-i"}, kubeFlags(kubeconfig, namespace)...)
	return append(a, pod, "-c", postgresContainer, "--")
}

type podList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			Phase             string `json:"phase"`
			ContainerStatuses []struct {
				Name  string `json:"name"`
				Ready bool   `json:"ready"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

// pickPod returns the name of the single running, ready postgres pod in the
// `kubectl get pods -o json` document.
func pickPod(doc []byte) (string, error) {
	var l podList
	if err := json.Unmarshal(doc, &l); err != nil {
		return "", errPod("the postgres pod list is not readable", "")
	}
	var ready []string
	for _, it := range l.Items {
		if it.Status.Phase != "Running" {
			continue
		}
		for _, c := range it.Status.ContainerStatuses {
			if c.Name == postgresContainer && c.Ready {
				ready = append(ready, it.Metadata.Name)
			}
		}
	}
	if len(ready) != 1 {
		return "", errPod(fmt.Sprintf("expected one running postgres pod, found %d (of %d matching)", len(ready), len(l.Items)), "")
	}
	return ready[0], nil
}

// exitCause renders a process error as its exit status only.
func exitCause(err error) string {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.Error()
	}
	return "the command could not be started"
}
