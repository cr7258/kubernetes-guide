package fast

#Env: {
	name:       string
	value?:     string
	valueFrom?: #FieldSelector
}
#FieldSelector: {
	fieldRef: {
		fieldPath: string
	}
}

//快速nginx工作负载
#serviceType: "ClusterIP" | "NodePort" | "LoadBalancer"
#input: {
	name:          string & "myngx"
	namespace?:    string | *"default"
	replicas:      int | *1
	port:          int | *80
	serviceEnable: bool | *true
	serviceType:   #serviceType | *"ClusterIP"
	nodePort:      int & >30000
	env?: [...#Env]

}
input: #input & {
	name:        "myngx"
	serviceType: "ClusterIP"
	nodePort:    30010
	env: [
		{
			name: "age", value: "19"
		},
		{
			name: "NODE_NAME"
			valueFrom:
				fieldRef:
					fieldPath: "spec.nodeName"
		},
	]
}

output: {
deployment: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name: input.name
		if input.namespace != _|_ {
			namespace: input.namespace
		}

	}
	spec: {
		replicas: input.replicas
		selector: {
			matchLabels: {
				app: input.name
			}
		}
		template: {
			metadata: {
				labels: {
					app: input.name
				}
			}
			spec:
				containers: [
					{
						name:  "nginx"
						image: "nginx:1.18-alpine"
						ports: [
							{
								containerPort: input.port
							},
						]
						if input.env != _|_ {
							//环境变量
							env: input.env
						}
					},
				]
		}
	}
}

if input.serviceEnable {
	service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			name: input.name + "svc"
			if input.namespace != _|_ {
				namespace: input.namespace
			}
		}
		spec: {
			type: input.serviceType
			ports: [
				{
					port:       input.port
					targetPort: input.port
					if input.serviceType == "NodePort" {
						nodePort: input.nodePort
					}
				},
			]
			selector: {
				app: input.name
			}
		}
	}
}
}
