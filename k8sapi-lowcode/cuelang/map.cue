#container: {
	name?: string
	image: string
}
#containers: [...#container]
#metadata: {
	name:      string
	namespace: string | *"default"
	labels?: [string]: string
}

//这是用户提交的
param: {
	"name": "abc"
	"containers": [
		{
			"name":  "nginx"
			"image": "nginx:1.18-alpine"
		},
	]
}

pod: {
	"apiVersion": "v1"
	"kind":       "Pod"
	"metadata":   #metadata & {
		name: param.name
		labels: {
			"author": "jtthink"
		}
	}
	"spec": {
		if param.containers != _|_ {
			"containers": #containers & [
					for _, v in param.containers {
					v
				},
			]
		}
	}
}
