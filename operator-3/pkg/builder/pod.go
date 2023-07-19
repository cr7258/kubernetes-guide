package builder

import (
	"context"
	"fmt"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"operator-3/pkg/apis/task/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"
)

type PodBuilder struct {
	task *v1alpha1.Task // 任务对象
	client.Client
}

// 构造函数
func NewPodBuilder(task *v1alpha1.Task, client client.Client) *PodBuilder {
	return &PodBuilder{task: task, Client: client}
}

// 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
const (
	//入口镜像  harbor
	EntryPointImage              = "docker.io/shenyisyn/entrypoint:v1.1"
	TaskPodPrefix                = "task-pod-" //Task对应的POD名称前缀
	AnnotationTaskOrderKey       = "taskorder" //要创建的 注解名称
	AnnotationTaskOrderInitValue = "0"
	AnnotationExitOrder          = "-1" //退出step用的Order标识
)

// 设置Init容器
func (pb *PodBuilder) setInitContainer(pod *v1.Pod) {
	pod.Spec.InitContainers = []v1.Container{
		{
			Name:            pod.Name + "init",
			Image:           EntryPointImage, //这里改成了常量
			ImagePullPolicy: v1.PullIfNotPresent,
			Command:         []string{"cp", "/app/entrypoint", "/entrypoint/bin"},
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "entrypoint-volume",
					MountPath: "/entrypoint/bin",
				},
			},
		},
	}
}

const (
	EntryPointVolume     = "entrypoint-volume"     //入口程序挂载
	JtthinkScriptsVolume = "jtthink-inner-scripts" //script属性 存储卷
	PodInfoVolume        = "podinfo"               //存储POD信息  用于dowardApi
)

// 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func (pb *PodBuilder) setPodVolumes(pod *v1.Pod) {
	volumes := []v1.Volume{
		{
			Name: EntryPointVolume,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: JtthinkScriptsVolume,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: PodInfoVolume,
			VolumeSource: v1.VolumeSource{
				DownwardAPI: &v1.DownwardAPIVolumeSource{
					Items: []v1.DownwardAPIVolumeFile{
						{
							Path: "order",
							FieldRef: &v1.ObjectFieldSelector{
								FieldPath: "metadata.annotations['taskorder']",
							},
						},
					},
				},
			},
		},
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, volumes...)
}

// 设置 POD元信息 包含 注解
func (pb *PodBuilder) setPodMeta(pod *v1.Pod) {
	pod.Namespace = pb.task.Namespace
	//pod.Name=TaskPodPrefix+pb.task.Name // pod名称
	pod.GenerateName = TaskPodPrefix + pb.task.Name + "-"
	pod.Spec.RestartPolicy = v1.RestartPolicyNever //从不 重启
	pod.Labels = map[string]string{
		"type":     "taskpod",
		"taskname": pb.task.Name,
	}
	pod.Annotations = map[string]string{
		AnnotationTaskOrderKey: AnnotationTaskOrderInitValue,
	}
}

// 判断  Task对应的Pod是否存在， 如果存在 返回POD
func (pb *PodBuilder) getChildPod() (*v1.Pod, error) {
	pods := &v1.PodList{}
	err := pb.Client.List(context.Background(), pods,
		client.HasLabels{"type", "taskname"},
		client.InNamespace(pb.task.Namespace))
	if err != nil { //没取到POD，则要进入创建流程
		return nil, err
	}
	for _, pod := range pods.Items {
		for _, own := range pod.OwnerReferences {
			if own.UID == pb.task.UID {
				return &pod, err
			}
		}
	}
	return nil, fmt.Errorf("found no task-pod")

}

// 设置容器，注意，增加了error返回值，
func (pb *PodBuilder) setContainer(index int, step v1alpha1.TaskStep) (v1.Container, error) {
	// 这里要强烈注意：step.Command必须要设置，如果没设置则通过http 去远程取。取不到直接报错
	command := step.Command // 取出它 原始的command ,是个 string切片
	if step.Script == "" {
		fmt.Println(step.Name, "用的是普通模式")
		if len(command) == 0 { //没有写 command  . 需要从网上去解析
			ref, err := name.ParseReference(step.Image, name.WeakValidation)
			if err != nil {
				return step.Container, err
			}
			//从缓存获取
			var getImage *Image
			if v, ok := ImageCache.Get(ref); ok { //代表 缓存是有的
				getImage = v.(*Image)
			} else { //缓存没有的情况下
				img, err := ParseImage(step.Image) //解析镜像
				if err != nil {
					return step.Container, err
				}
				ImageCache.Add(img.Ref, img) //加入缓存
				getImage = img
			}
			//懒得解析， 暂时先写死 OS=Linux/amd64
			tempOs := "linux/amd64"
			if imgObj, ok := getImage.Command[tempOs]; ok {
				command = imgObj.Command
				if len(step.Args) == 0 { // 覆盖args （假设有的话)
					step.Args = imgObj.Args
				}
			} else {
				return step.Container, fmt.Errorf("error image command")
			}

		}
		args := step.Args //取出它原始的 args

		step.Container.ImagePullPolicy = v1.PullIfNotPresent //强迫设置拉取策略
		step.Container.Command = []string{"/entrypoint/bin/entrypoint"}
		step.Container.Args = []string{
			"--wait", "/etc/podinfo/order",
			"--waitcontent", strconv.Itoa(index + 1),
			"--out", "stdout", // entrypoint 中 写上stdout 就会定向到标准输出
			"--command",
		}
		// "sh -c"
		step.Container.Args = append(step.Container.Args, strings.Join(command, " "))
		step.Container.Args = append(step.Container.Args, args...)

	} else {
		fmt.Println(step.Name, "用的是script模式")
		// 代表设置了script 。 此时 无视 command 和args

		step.Container.Command = []string{"sh"} //写死的
		step.Container.Args = []string{"-c", fmt.Sprintf(`
scriptfile="/jtthink/scripts/%s";
touch ${scriptfile} && chmod +x ${scriptfile};
echo "%s" > ${scriptfile}; 
/entrypoint/bin/entrypoint --wait /etc/podinfo/order --waitcontent %d   --out stdout  --encodefile ${scriptfile};
`, step.Name, EncodeScript(step.Script), index+1)}
	}
	//设置挂载点
	step.Container.VolumeMounts = []v1.VolumeMount{
		{
			Name:      EntryPointVolume,
			MountPath: "/entrypoint/bin",
		},
		{
			Name:      JtthinkScriptsVolume, //设置 script挂载卷，不管有没有设置
			MountPath: "/jtthink/scripts",
		},
		{
			Name:      PodInfoVolume,
			MountPath: "/etc/podinfo",
		},
	}
	return step.Container, nil
}

// 任务王前进
func (pb *PodBuilder) forward(ctx context.Context, pod *v1.Pod) error {
	if pod.Status.Phase == v1.PodSucceeded {
		return nil
	}
	// Order值 ==-1  代表 有一个step出错了。不做处理。
	if pod.Annotations[AnnotationTaskOrderKey] == AnnotationExitOrder {
		return nil
	}
	order, err := strconv.Atoi(pod.Annotations[AnnotationTaskOrderKey])
	if err != nil {
		return err
	}
	// 长度相等 ，代表已经到了最后一个 。不需要前进
	if order == len(pod.Spec.Containers) {
		return nil
	}
	//代表 当前的容器可能在等待  或者正在运行
	containerState := pod.Status.ContainerStatuses[order-1].State
	if containerState.Terminated == nil {
		return nil
	} else {
		//代表非正常退出
		if containerState.Terminated.ExitCode != 0 {
			//吧Order 值改成 -1
			pod.Annotations[AnnotationTaskOrderKey] = AnnotationExitOrder
			return pb.Client.Update(ctx, pod)
			//pod.Status.Phase=v1.PodFailed
			//return pb.Client.Status().Update(ctx,pod)
		}
	}
	order++
	pod.Annotations[AnnotationTaskOrderKey] = strconv.Itoa(order)
	return pb.Client.Update(ctx, pod)
}

// 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
// 构建 创建出 对应的POD
func (pb *PodBuilder) Build(ctx context.Context) error {
	//判断 POD是否存在
	getPod, err := pb.getChildPod()
	if err == nil { //代表POD已经被创建
		//这代表启动阶段
		if getPod.Status.Phase == v1.PodRunning {
			//起始状态
			if getPod.Annotations[AnnotationTaskOrderKey] == AnnotationTaskOrderInitValue {
				getPod.Annotations[AnnotationTaskOrderKey] = "1" //写死  。故意的
				return pb.Client.Update(ctx, getPod)
			} else {
				if err := pb.forward(ctx, getPod); err != nil {
					return err
				}
			}
		}
		fmt.Println("新状态", getPod.Status.Phase)
		fmt.Println("order是", getPod.Annotations[AnnotationTaskOrderKey])
		return nil
	}

	//创建
	newPod := &v1.Pod{}
	pb.setPodMeta(newPod)       //设置元信息 ，如name,namespace 和annotations(重要的一匹)
	pb.setInitContainer(newPod) // 设置 initContainers
	c := []v1.Container{}       // 容器切片
	for index, step := range pb.task.Spec.Steps {
		getContainer, err := pb.setContainer(index, step)
		if err != nil {
			return err
		}
		c = append(c, getContainer) //修改容器的command和args ----后面还要改
	}
	newPod.Spec.Containers = c
	pb.setPodVolumes(newPod) // 设置pod数据卷--重要，包含了downwardAPI 和emptyDir
	//设置owner
	newPod.OwnerReferences = append(newPod.OwnerReferences,
		metav1.OwnerReference{
			APIVersion: pb.task.APIVersion,
			Kind:       pb.task.Kind,
			Name:       pb.task.Name,
			UID:        pb.task.UID,
		})
	return pb.Create(ctx, newPod)
}

// 本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func (pb *PodBuilder) setStep(pod *v1.Pod) {
	pod.Annotations = map[string]string{
		"taskorder": "0",
	}

}

//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
