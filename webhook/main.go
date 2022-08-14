package main

import (
	"encoding/json"
	"k8s.io/klog/v2"
	"log"
	"myhook/lib"

	"io/ioutil"
	"k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
)

func main() {

	http.HandleFunc("/pods", func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.RequestURI)
		var body []byte
		if r.Body != nil {
			if data, err := ioutil.ReadAll(r.Body); err == nil {
				body = data
			}
		}
		//第二步
		reqAdmissionReview := v1.AdmissionReview{} //请求
		rspAdmissionReview := v1.AdmissionReview{  //响应 ---只构建了一部分
			TypeMeta: metav1.TypeMeta{
				Kind:       "AdmissionReview",
				APIVersion: "admission.k8s.io/v1",
			},
		}
		//第三步。 把body decode 成对象
		deserializer := lib.Codecs.UniversalDeserializer()
		if _, _, err := deserializer.Decode(body, nil, &reqAdmissionReview); err != nil {
			klog.Error(err)
			rspAdmissionReview.Response = lib.ToV1AdmissionResponse(err)
		} else {
			rspAdmissionReview.Response = lib.AdmitPods(reqAdmissionReview) //我们的业务
		}
		rspAdmissionReview.Response.UID = reqAdmissionReview.Request.UID
		respBytes, err := json.Marshal(rspAdmissionReview)
		if err != nil {
			klog.Error(err)
		} else {
			if _, err := w.Write(respBytes); err != nil {
				klog.Error(err)
			}
		}

	})
	tlsConfig := lib.Config{
		CertFile: "/etc/webhook/certs/tls.crt",
		KeyFile:  "/etc/webhook/certs/tls.key",
	}
	server := &http.Server{
		Addr:      ":443",
		TLSConfig: lib.ConfigTLS(tlsConfig),
	}
	server.ListenAndServeTLS("", "")

	//http.ListenAndServe(":8080",nil )
}
