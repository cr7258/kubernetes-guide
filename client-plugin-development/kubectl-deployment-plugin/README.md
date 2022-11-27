## 伸缩副本
交互式库：https://github.com/charmbracelet/bubbles
```bash
# 启动交互式界面
> go run main.go prompt
>>> list
名称             	副本数	创建时间        	最新事件
nginx            	3/3   	2022/10/21 18:20

# 伸缩副本
>>> scale nginx
请填写需要收缩的副本数(0-20之间)

> 5
(按ECS退出)
副本收缩成功

>>> list
名称             	副本数	创建时间        	最新事件
nginx            	5/5   	2022/10/21 18:20	Scaled up replica set
                 	      	                	nginx-8f458dc5b to 5
```

## 查看详情
```bash
>>> get redis2
  元信息
  标签
  注解
  标签选择器
  pod模板
  状态
  副本数
  全部
  *事件*
» *查看POD*

按Q退出
POD名称                 IP              状态    节点                 
redis2-74f9d6489f-kflsn 10.244.0.181    Running pool-wpenmh9h4-7htb2   

```