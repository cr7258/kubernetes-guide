## 安装 WasmEdge

```bash
curl -sSf https://raw.githubusercontent.com/WasmEdge/WasmEdge/master/utils/install.sh | bash
```

确认 Wasmedge 安装成功。

```bash
root@instance-2:~/# wasmedge -v
wasmedge version 0.12.1
```
## 安装支持 WasmEdge 的 crun 二进制文件

crun 项目已经内置了对 WasmEdge 的支持。 目前，最简单的方法是自己从源代码构建它。首先，让我们确保 crun 在你的 Ubuntu 20.04 上安装了依赖包。 

```bash
sudo apt update
sudo apt install -y make git gcc build-essential pkgconf libtool \
     libsystemd-dev libprotobuf-c-dev libcap-dev libseccomp-dev libyajl-dev \
     go-md2man libtool autoconf python3 automake
```

接下来，配置、构建及安装一个支持 WasmEdge 的 crun 二进制文件。

```bash
git clone https://github.com/containers/crun
cd crun
./autogen.sh
./configure --with-wasmedge
make
sudo make install
```

确认 crun 安装成功，必须要有 +WASM:wasmedge。

```bash
root@instance-2:~/crun# crun -v
crun version 1.8.5.0.0.0.23-3856
commit: 385654125154075544e83a6227557bfa5b1f8cc5
rundir: /run/crun
spec: 1.0.0
+SYSTEMD +SELINUX +APPARMOR +CAP +SECCOMP +EBPF +WASM:wasmedge +YAJL
```

## 安装 Containerd

使用以下命令在您的系统上安装 containerd。

```bash
export VERSION="1.5.7"
echo -e "Version: $VERSION"
echo -e "Installing libseccomp2 ..."
sudo apt install -y libseccomp2
echo -e "Installing wget"
sudo apt install -y wget

wget https://github.com/containerd/containerd/releases/download/v${VERSION}/cri-containerd-cni-${VERSION}-linux-amd64.tar.gz
wget https://github.com/containerd/containerd/releases/download/v${VERSION}/cri-containerd-cni-${VERSION}-linux-amd64.tar.gz.sha256sum
sha256sum --check cri-containerd-cni-${VERSION}-linux-amd64.tar.gz.sha256sum

sudo tar --no-overwrite-dir -C / -xzf cri-containerd-cni-${VERSION}-linux-amd64.tar.gz
sudo systemctl daemon-reload
```

将 containerd 配置为使用 crun 作为底层 OCI runtime。 此处需要修改 /etc/containerd/config.toml 文件。

```bash
sudo mkdir -p /etc/containerd/
sudo bash -c "containerd config default > /etc/containerd/config.toml"
wget https://raw.githubusercontent.com/second-state/wasmedge-containers-examples/main/containerd/containerd_config.diff
sudo patch -d/ -p0 < containerd_config.diff
```

启动 containerd 服务。

```bash
sudo systemctl start containerd
```

确认 containerd 运行成功。

```bash
root@instance-2:~# systemctl status containerd.service
● containerd.service - containerd container runtime
     Loaded: loaded (/etc/systemd/system/containerd.service; disabled; vendor preset: enabled)
     Active: active (running) since Mon 2023-06-05 03:50:46 UTC; 4s ago
       Docs: https://containerd.io
    Process: 96812 ExecStartPre=/sbin/modprobe overlay (code=exited, status=0/SUCCESS)
   Main PID: 96815 (containerd)
      Tasks: 8
     Memory: 18.5M
        CPU: 120ms
     CGroup: /system.slice/containerd.service
             └─96815 /usr/local/bin/containerd

Jun 05 03:50:46 instance-2 containerd[96815]: time="2023-06-05T03:50:46.665538548Z" level=info msg=serving... address=/run/containerd/containerd.sock.ttrpc
Jun 05 03:50:46 instance-2 containerd[96815]: time="2023-06-05T03:50:46.665764015Z" level=info msg=serving... address=/run/containerd/containerd.sock
Jun 05 03:50:46 instance-2 systemd[1]: Started containerd container runtime.
Jun 05 03:50:46 instance-2 containerd[96815]: time="2023-06-05T03:50:46.665559121Z" level=info msg="Start subscribing containerd event"
Jun 05 03:50:46 instance-2 containerd[96815]: time="2023-06-05T03:50:46.667714504Z" level=info msg="Start recovering state"
Jun 05 03:50:46 instance-2 containerd[96815]: time="2023-06-05T03:50:46.668015794Z" level=info msg="Start event monitor"
Jun 05 03:50:46 instance-2 containerd[96815]: time="2023-06-05T03:50:46.668280745Z" level=info msg="Start snapshots syncer"
Jun 05 03:50:46 instance-2 containerd[96815]: time="2023-06-05T03:50:46.668440128Z" level=info msg="Start cni network conf syncer"
Jun 05 03:50:46 instance-2 containerd[96815]: time="2023-06-05T03:50:46.668589738Z" level=info msg="Start streaming server"
Jun 05 03:50:46 instance-2 containerd[96815]: time="2023-06-05T03:50:46.669911123Z" level=info msg="containerd successfully booted in 0.059048s"
```

## 本地编写 Rust 项目

### 安装 Rust

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

### 编写代码

我们使用 Cargo 创建一个新项目。

```bash
cargo new hello_world
cd hello_world
```

在 src/main.rs 中编写简单的代码，每隔 5 秒打印 Hello, Rust。

```rust
use std::time::Duration;
use std::thread::sleep;
fn main() {
    loop {
        println!("Hello, Rust");
        sleep(Duration::from_secs(5));
    }
}
```

### 构建 WASM 字节码

生成的 wasm 字节码文件位于 target/wasm32-wasi/release/hello_world.wasm。

```bash
rustup target add wasm32-wasi
cargo build --target wasm32-wasi --release
```

为 wasm 字节码文件添加可执行权限。

```bash
chmod +x target/wasm32-wasi/release/hello_world.wasm
```

## 构建镜像

安装 nerdctl 和 buildkit 工具来构建镜像。

```bash
wget https://github.com/containerd/nerdctl/releases/download/v1.4.0/nerdctl-1.4.0-linux-amd64.tar.gz
tar -xzvf nerdctl-1.4.0-linux-amd64.tar.gz
chmod + nerdctl
mv nerdctl /usr/local/bin/
```

```bash

在 target/wasm32-wasi/release/ 文件夹下创建一个名为 Dockerfile 的文件，内容如下:

```Dockerfile
FROM scratch
ADD wasi_example_main.wasm /
CMD ["/hello_world.wasm"]
```

然后在 arget/wasm32-wasi/release/ 目录下，执行以下命令：

```bash


```
## 参考资料
- [用 Kubernetes 管理 WasmEdge 应用](https://wasmedge.org/book/zh/kubernetes.html)
- [Kwasm](https://kwasm.sh/quickstart/)
- [kind-crun-wasm](https://github.com/Liquid-Reply/kind-crun-wasm)