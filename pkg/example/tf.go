package example

var TF_EXAMPLE = `
kind: MLApp
metadata:
  name: tfexample
  namespace: mapp
  labels: # Will be applayed to each resource
    testKey: testValue
spec:
  tasks:
  - name: model
    labels:
      testModelKey: testModelValue  # Will be applayed to each resource
    resources:
    - name: worker
      labels:
        testWorkerKey: testWorkerValue  # Will be applayed to each resource
      replicas: 2
      minAvailable: 2
      restartPolicy: Never
      maxRestartCount: 0
      images:
        gpu: image-gpu
        cpu: image-cpu
      command: python
      workdir: directory
      args: "--log-dir=$TRAINING_DIR"
      env:
      - name: TEST_ENV_V1
        value: v1
      - name: PYTHONPATH
        value: /usr/local
      resources:
        accelerators:
          gpu: 1
          dedicated_gpu: true
        requests:
          cpu: 100m
          memory: 1Gi
        limits:
          cpu: 100m
          memory: 1Gi
      port: 9000
      volumes:
      - name: lib
      - name: training
        subPath: build-1
    - name: ps
      labels:
        testPsKey: testPsValue  # Will be applayed to each resource
      replicas: 1
      minAvailable: 1
      restartPolicy: Never
      maxRestartCount: 0
      images:
        cpu: image-cpu
      command: python
      workdir: directory
      port: 9000
      env:
      - name: TEST_ENV_V2
        value: v2
      volumes:
      - name: training
  uix:
    - name: jupyter
      displayName: Jupyter
      images:
        gpu: image-gpu
        cpu: image-cpu
      resources:
        accelerators:
          gpu: 1
        requests:
          cpu: 100m
          memory: 1Gi
        limits:
          cpu: 100m
          memory: 1Gi
      ports:
        - port: 80
          targetPort: 8082
          protocol: TCP
          name: http
      volumes:
        - name: lib
  volumes:
    - name: lib
      isLibDir: true
      isTrainLogDir: false
      mountPath: /workspace/lib
      subPath: lib
      clusterStorage: test
      hostPath:
        path: /test
    - name: training
      isLibDir: false
      isTrainLogDir: true
      mountPath: /workspace/training
      subPath: training
      clusterStorage: test
      hostPath:
        path: /test
`
