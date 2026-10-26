package kubernetes

import (
	"encoding/json"
	"fmt"
	"github.com/wsq2333/malta/internal/domain"
	"time"
)

type object = map[string]any

func Name(r domain.Run) string { return "malta-" + r.Attempt.ID }
func labels(r domain.Run) object {
	return object{"app.kubernetes.io/managed-by": "malta", "malta.run": r.ID, "malta.attempt": r.Attempt.ID, "malta.plan": r.Plan.Hash[:32]}
}
func (e *Executor) Objects(r domain.Run, timeout time.Duration, maxBytes int64) (object, object) {
	name := Name(r)
	raw, _ := json.Marshal(r)
	metadata := object{"name": name, "namespace": e.Config.Namespace, "labels": labels(r)}
	cm := object{"apiVersion": "v1", "kind": "ConfigMap", "metadata": metadata, "immutable": true, "data": object{"run.json": string(raw)}}
	resources := object{"cpu": fmt.Sprintf("%dm", e.Config.CPUMillis), "memory": fmt.Sprint(e.Config.MemoryBytes)}
	pod := object{
		"serviceAccountName": e.Config.ServiceAccount, "automountServiceAccountToken": false, "restartPolicy": "Never",
		"terminationGracePeriodSeconds": 30, "nodeSelector": object{"kubernetes.io/hostname": e.Config.Node},
		"securityContext": object{"runAsNonRoot": true, "runAsUser": 10001, "runAsGroup": 10001, "fsGroup": 10001, "seccompProfile": object{"type": "RuntimeDefault"}},
		"containers":      []any{object{"name": "runner", "image": e.Config.Image, "imagePullPolicy": "IfNotPresent", "command": []string{"/usr/local/bin/malta"}, "args": []string{"worker", "--run", "/etc/malta/run.json", "--results", "/results", "--timeout", timeout.String(), "--max-bytes", fmt.Sprint(maxBytes)}, "resources": object{"requests": resources, "limits": resources}, "securityContext": object{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": object{"drop": []string{"ALL"}}}, "env": []any{object{"name": "MALTA_POD_UID", "valueFrom": object{"fieldRef": object{"fieldPath": "metadata.uid"}}}}, "volumeMounts": []any{object{"name": "bench", "mountPath": "/bench"}, object{"name": "plan", "mountPath": "/etc/malta", "readOnly": true}, object{"name": "results", "mountPath": "/results"}, object{"name": "tmp", "mountPath": "/tmp"}}}},
		"volumes":         []any{object{"name": "bench", "persistentVolumeClaim": object{"claimName": e.Config.BenchmarkPVC}}, object{"name": "results", "persistentVolumeClaim": object{"claimName": e.Config.ResultsPVC}}, object{"name": "plan", "configMap": object{"name": name}}, object{"name": "tmp", "emptyDir": object{"sizeLimit": "64Mi"}}},
	}
	job := object{"apiVersion": "batch/v1", "kind": "Job", "metadata": metadata, "spec": object{"parallelism": 1, "completions": 1, "backoffLimit": 0, "activeDeadlineSeconds": int64(timeout.Seconds()) + 45, "podReplacementPolicy": "Failed", "template": object{"metadata": object{"labels": labels(r)}, "spec": pod}}}
	return cm, job
}
