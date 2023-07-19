package builder

import (
	"fmt"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	lru "github.com/hashicorp/golang-lru"
	"log"
	"strings"
)
//镜像缓存
var ImageCache *lru.Cache

func InitImageCache(size int )  {
	c,err:=lru.New(size)
	if err!=nil{
		log.Fatalln(err)
	}
	ImageCache=c
}
type ImageCommand struct {
	Command []string // 注意是切片   对应docker 的entrypoint
	Args []string   //注意是切片  对应docker 的cmd
}
func(ic *ImageCommand) String() string{
	return fmt.Sprintf("Command是:%s,Args是:%s",strings.Join(ic.Command," "),strings.Join(ic.Args," "))
}
//主对象
type Image struct {
	Ref name.Reference //增加了一个 。 缓存里直接用这个 作为key，更方便
	Name string  //譬如 alpine:3.12
	Digest v1.Hash //唯一的 hash
	Command map[string]*ImageCommand  //  map的key  譬如 Linux/amd64
}
func(i *Image) AddCommand(os,arch string ,cmds []string,args []string  ){
	key:=fmt.Sprintf("%s/%s",os,arch)
	 i.Command[key]=&ImageCommand{
	 	Command: cmds,
	 	Args: args,
	 }
}

//初始化函数
func NewImage(name string ,digest v1.Hash,ref name.Reference ) *Image{
		return &Image{
			Name:name,
			Ref: ref,
			Digest: digest,
			Command: make(map[string]*ImageCommand),
		}
}

//解析 image 镜像
func ParseImage(img string ) (*Image,error){
	ref,err:=name.ParseReference(img,name.WeakValidation) //使用非严格模式
	if err!=nil{
		return nil,err
	}

	des,err:=remote.Get(ref)// 获取镜像描述信息
	if err!=nil{
		return nil,err
	}
	//初始化我们的业务 Image 对象
	imgBuilder:= NewImage(ref.Name(),des.Digest,ref)
	//image 类型
	if des.MediaType.IsImage(){
		img, err := des.Image()
		if err != nil {
			return nil, err
		}
		conf,err := img.ConfigFile()
		if err != nil {
			return nil, err
		}
		imgBuilder.AddCommand(conf.OS,conf.Architecture,conf.Config.Entrypoint,conf.Config.Cmd)
		return imgBuilder,nil
	}


	//下方是 index 模式
	idx, err := des.ImageIndex()
	if err != nil {
		return nil,err
	}
	mf, err :=idx.IndexManifest()

	if err != nil {
		return nil,err
	}
	for _, d := range mf.Manifests {
		img, err := idx.Image(d.Digest)
		if err != nil {
			return nil,err
		}
		conf,err:=img.ConfigFile()
		if err != nil {
			return nil,err
		}
		imgBuilder.AddCommand(conf.OS,conf.Architecture,conf.Config.Entrypoint,conf.Config.Cmd)
		//fmt.Println(cf.OS,"/",cf.Architecture,":",cf.Config.Entrypoint,cf.Config.Cmd)
	}
	return  imgBuilder,nil
}