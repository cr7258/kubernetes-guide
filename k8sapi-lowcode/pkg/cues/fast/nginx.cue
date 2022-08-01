package fast


#FieldSelector:{
	fieldRef:{
		 fieldPath: string
	}
}

//快速nginx工作负载
#serviceType: "ClusterIP" | "NodePort" | "LoadBalancer"
#input: {
	 // 工作负载名称
   name: string | *"myngx"
   // 命名空间
   namespace: string
   // 副本数
   replicas: int & >=1 | *1
   // 服务端口
   port: int | *80
   // 是否同步创建服务
   serviceEnable: bool | *true
   // 服务类型
   serviceType: #serviceType | *"ClusterIP"
   // nodePort端口
   nodePort: int & >30000
   // 当前表单版本
	 gvr: "k8s.jtthink.com_v1_fastnginxs"
}
#Env:{
	 // 环境变量名称
   name: string
   // 环境变量值
   value?: string
   // 环境变量引用
   valueFrom: #FieldSelector
}
input: #input
uiSchema: {

	"ui:order":[
            "name","namespace","port","replicas","serviceEnable",
            "serviceType","nodePort","gvr"
          ],
	"serviceType":{
		 "ui:hidden":"{{ rootFormData.serviceEnable==false}}",
		 "ui:options": {
		 	  style :{
		 	     "margin-left": "10px"
		 	  }
		 }
	},
	 "nodePort":{
		"ui:hidden":"{{ rootFormData.serviceEnable==false || rootFormData.serviceType!='NodePort'}}",
		"ui:options": {
		 	  style :{
		 	     "margin-left": "10px"
		 	  }
		 }
	}
}
output: {
 deployment:{
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
			name: input.name
			if input.namespace !=_|_{
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
							 name: "nginx"
							 image: "nginx:1.18-alpine"
							 ports: [
									 {
											containerPort: input.port
									 }
							 ]
							 if input.env!=_|_{
 									  //环境变量
 									  env: input.env
							 }
						 }
				]
		 }
	 }
}

if input.serviceEnable{
 service:{
	apiVersion: "v1"
  kind: "Service"
	metadata:{
		 name: input.name+"svc"
		 if input.namespace !=_|_{
		 namespace: input.namespace
		 }
	}
	spec:{
	 type: input.serviceType
	 ports: [
		 {
			 port: input.port
			 targetPort: input.port
			 if input.serviceType=="NodePort"{
			    nodePort:	input.nodePort
			 }
		 }
	 ]
	 selector:{
	 	 app: input.name
	 }
  }
 }
}

}

