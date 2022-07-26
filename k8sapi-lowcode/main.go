package main

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/gin-gonic/gin"
	"k8sapi-lowcode/pkg/utils"
)

type InputPod struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

func main() {

	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		pod := &InputPod{}
		if err := c.ShouldBindJSON(pod); err != nil {
			c.AbortWithStatusJSON(400, gin.H{"err": err.Error()})
			return
		}
		cc := cuecontext.New()
		cv := cc.CompileBytes(utils.MustLoadFile("./test.cue"))
		cv = cv.FillPath(cue.ParsePath("param"), cc.Encode(pod))
		b, err := cv.LookupPath(cue.ParsePath("pod")).MarshalJSON()
		if err != nil {
			c.AbortWithStatusJSON(400, gin.H{"err": err.Error()})
			return
		}
		c.Writer.Header().Add("content-type", "application/json")
		c.Writer.Write(b)
	})

	r.Run(":8080")
}
