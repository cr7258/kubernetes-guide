let defaultns = "abc"

#metadata:{
   name: string
   namespace: string | *defaultns
}

#container: {
	name?: string
	image: string
}

#containers: [...#container]

containers: #containers & [
	{
		"image": "nginx:1.18-alpine"
	},
	{
		"image": "tomcat",
		"name": "myapp"
	}
]
metadata: #metadata & {
 "name": "nginx"
}

param: {}

pod: {
  "apiVersion": param.apiVersion,
  "kind": param.kind,
  "metadata":metadata,
  "spec": {
    "containers": containers
  }
}
