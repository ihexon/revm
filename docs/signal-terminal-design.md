# revm 信号与终端行为设计

状态：设计提案，待实现。

本方案定义命令执行、终端输入、宿主信号和 VM 停止之间的关系。核心约定是：每次执行拥有独立的命令会话，终端输入作用于 guest 前台任务，取消请求清理本次执行，创建 VM 的命令负责最终关闭 VM。

文中的新参数和行为是目标接口；当前使用方式仍以 [run](run.md)、[attach](attach.md)、[podman](podman.md) 和 [ctl](ctl.md) 文档为准。session、instance 和持久数据的所有权遵循 [资源生命周期设计](resource-lifecycle-design.md)。停止 VM 不等于删除 session 记录或持久数据。

## 当前行为与目标

当前 run 的命令由 guest-agent 在启动过程中执行，终端通过 libkrun console 连接；普通 attach 使用 guest-control，PTY attach 使用 SSH shell。三条路径的执行、退出状态和终端管理尚未统一。

当前 run 和 podman 宿主进程收到的 SIGINT 和 SIGTERM 都进入 VM shutdown 路径；首次信号请求关机，第二次信号强制停止。PTY 下的按键是否产生宿主信号取决于宿主终端模式，不能用这套规则统一解释所有 Ctrl-C。

目标是让 run 和 attach 共用执行会话，统一流式输入、退出结果、信号和 PTY 行为，同时保留它们对 VM 的不同权限。

## 所有权与执行范围

session ID 表示稳定名称，instance ID 表示本次 VM 运行，exec ID 表示该 instance 内的一次命令执行。执行控制必须同时定位 instance 和 exec，不能将旧实例的取消请求应用到同名新实例。

| 命令 | 拥有的资源 | 结束和取消的范围 |
| --- | --- | --- |
| run | 新 VM instance 和一次命令执行 | 收集命令结果，清理工作负载，关闭自己创建的 VM |
| attach | 已有 instance 内的一次命令执行 | 清理本次执行及其后代，保留 VM 和其他执行 |
| podman | 提供容器服务的 VM instance | 请求服务和 VM 关闭，保留持久容器数据 |
| stop | 指定 instance 的停止请求 | 等待目标停止或报告失败，不取得数据删除权限 |

attach 延续现有语义，在已有 VM 中启动新命令；不接管 run 的原始进程，也不自动恢复此前断开的终端。持久执行、重新连接和 detach 属于后续独立接口。

## 终端选项

run 和 attach 使用相同的选项：

~~~text
--tty=auto|always|never
~~~

默认值为 auto。stdin 和 stdout 都连接终端，且 revm 位于 stdin 所属终端的前台进程组时，分配 guest PTY。stderr 是否重定向不参与检测。

| 模式 | Guest 端 | Host 端 |
| --- | --- | --- |
| auto | 满足条件时分配 PTY，否则使用三个独立标准流 | 仅交互终端路径进入 raw 模式 |
| always | 强制分配 PTY，即使 stdin 是管道 | 只有 stdin 是本地前台终端时才进入 raw 模式；后台终端不能被静默接管 |
| never | 独立的 stdin、stdout、stderr | 不修改 termios，不进行文本或换行转换 |

stdin 默认接通，不要求额外的交互标志。PTY 输出合并 stdout 和 stderr，并可能经过 guest 终端的换行处理；需要独立标准流或二进制传输时使用 never。

示例：

~~~sh
revm run --id shell --tty=auto -- sh
revm attach --id dev --tty=always -- top
printf 'hello\n' | revm attach --id dev --tty=never -- cat
~~~

指定命令时执行该命令，不能因 PTY 模式而忽略 argv。attach 省略命令时启动 /bin/sh；run 仍要求命令。旧 --pty 作为 --tty=always 的兼容入口保留，冲突选项在启动前报错。

分配 PTY 时传递初始窗口尺寸和 TERM。无可用尺寸时使用 80 列、24 行；无 TERM 时使用文档化的回退值 xterm-256color。SIGWINCH 更新尺寸，连续更新可合并为最后一次。

## 输入与信号规则

| 事件 | 行为 |
| --- | --- |
| PTY 下输入 Ctrl-C | 输入字节交给 guest PTY，由 guest 终端和前台任务处理 |
| PTY 下输入 Ctrl-\\ | 交给 guest 终端的 quit 行为处理 |
| PTY 下输入 Ctrl-Z | 交给 guest 终端和 shell 的作业控制处理 |
| PTY 下输入 Ctrl-D | 作为输入字节交给 guest；是否产生 EOF 由 guest 终端模式决定 |
| 非 PTY stdin EOF | 只关闭 guest stdin，继续读取输出并等待结果 |
| Host 收到 SIGINT、SIGTERM、SIGHUP 或 SIGQUIT | 请求取消当前操作，保留原始信号原因，进入有界清理 |
| Host 收到 SIGWINCH | 更新 guest PTY 尺寸，不取消执行 |
| Host 收到 SIGTSTP 或 SIGCONT | 执行宿主作业控制和终端恢复规则 |

宿主 raw 模式不解释 Ctrl-C 等控制字符。guest 终端在启用 ISIG 时为相应字符生成信号；host 不再额外发送同一个 signal RPC，避免双重中断。guest 应用关闭 ISIG 后，控制字符仍由应用自行解释。[termios 规则](https://man7.org/linux/man-pages/man3/termios.3.html)

在 guest shell 中运行 sleep 后按 Ctrl-C，应返回 guest shell，VM 保持运行。连续输入两个 Ctrl-C 也仍然是 guest 输入，不解释为强制关闭 VM。明确关闭整个 VM 使用 stop，或向拥有 VM 的宿主进程发送取消信号。

非 PTY 模式下，如果本地终端将 Ctrl-C 转换成宿主 SIGINT，则按宿主取消处理。启动阶段尚未接通 guest 终端时同样如此。

PTY 不支持与普通管道等价的写侧半关闭。强制 PTY 模式下遇到宿主输入 EOF，停止读取本地输入，但不注入 Ctrl-D、不关闭 PTY master，也不立即取消执行；继续等待 guest 退出或显式取消。依赖 EOF 结束的管道程序应使用 never。

## 取消与停止时序

run、attach、podman 和 stop 提供 --stop-timeout，拟定默认值为 10s，表示优雅停止预算。允许 0，表示直接进入强制阶段；负值无效。超时从取消请求被接受时计算，不因后续阶段或重复请求重新开始。

~~~text
启动中或运行中
    │ 首次取消请求
    ▼
优雅停止
    │ 超过 stop-timeout，或再次收到取消信号
    ▼
强制清理
    │ 收集结果和剩余输出，恢复终端
    ▼
结束或报告清理未完成
~~~

运行中的命令先收到原始 SIGINT、SIGTERM、SIGHUP 或 SIGQUIT。内部取消或连接丢失没有原始信号时使用 SIGTERM。普通信号投递只表示通知工作负载；cancel 还要求监督器在期限内完成清理。

优雅阶段继续读取输出，并保留网络、管理 API 和执行控制通道。不能先取消全部 host service context，再尝试通过这些服务关闭 guest。

run 的工作负载停止和 VM 优雅关机共享同一个 10s 预算；工作负载提前结束后，剩余预算用于 VM 关机。podman 和 stop 使用相同预算等待服务及 VM 停止。attach 的预算只作用于本次执行。

超时或第二次宿主取消信号进入强制阶段，拟定额外预算为 3s，覆盖强制停止和最后的输出收尾。该阶段幂等，不重复销毁句柄，也不延长期限。目标是默认最多约 13s 完成取消，不对 CPU 调度或不可中断内核调用作实时保证。

run 可以最终强制停止自己创建的 VM；attach 不能升级为停止整个 VM。控制通道失效、无法确认 guest 清理时，attach 返回失败并记录清理未确认，由 guest 的断连清理逻辑继续处理。

若后端的阻塞 C 调用无法在强制阶段返回，单进程模型无法保证宿主命令及时结束。实现必须验证 libkrun 的停止能力；若不满足预算，应将 VMM 放入可终止的独立子进程，不能仅让计时器返回错误而继续无限等待后台任务。

启动期间尚无 guest 进程时，取消启动并释放已经取得的资源。执行结果已完成后到达的迟发信号不改写命令结果，但仍可加速尚未完成的 VM 清理。

## Guest 执行监督

每次执行由独立监督器负责启动、信号投递、后代清理、输出排空和结果收集。非 PTY 命令使用独立进程组；PTY 命令创建 session 和 controlling terminal，让 guest shell 管理前台进程组。

交互按键由 PTY 行规程作用于当前前台任务。取消整个执行时，监督器还需要终止该执行的 shell、后台任务和其他后代，不能只向最初的 PID 发信号。

进程组不能覆盖通过 setsid 等方式脱离的后代。完整清理以每次执行独立的 guest cgroup 为边界；正常结束和取消后都回收该边界内残留进程。工作负载不应能自行迁出该边界。仅实现 PGID 的阶段必须明确这一限制，不能宣称已经保证所有后代退出。

exec.CommandContext 的默认取消调用 Process.Kill，不能直接承担优雅退出和进程组清理。应使用独立监督逻辑或自定义取消函数，并由单一位置调用 Wait。[Go exec 行为](https://pkg.go.dev/os/exec#CommandContext)

连接丢失触发本次执行的取消，不能将 HTTP 请求 context 直接等同于整个执行生命周期。监督器在连接失效后仍负责有界清理和结果记录。显式 detach 才能改变这一所有权规则。

## 执行协议

run 和 attach 最终共用 guest-control 执行会话。下面定义消息语义，不约束具体帧编码；新协议需要版本协商，不直接改变当前 v1 的含义。

~~~text
host → guest
  start(request_id, argv, env, cwd, tty, term, size)
  stdin(data)
  stdin_eof
  signal(name)
  resize(rows, cols)
  cancel(reason, initial_signal, grace)

guest → host
  started(instance_id, exec_id)
  stdout(data) / stderr(data) / tty_output(data)
  exit(code 或 signal)
  error(kind, message)
  completed(cleanup_status, output_status)
~~~

命令使用 argv 传递，不隐式经过 shell。控制消息绑定 instance ID 和 exec ID，不能接受客户端随意指定 guest PID。跨平台信号使用名称，避免把 macOS 的数字编号直接用于 Linux。

start 使用请求标识去重，响应丢失不能导致重试后执行两次。started 之前发生取消时，通过同一请求标识终止待启动操作；若启动已经发生，服务端必须关联并清理该执行。

stdin 数据与 stdin_eof 保持顺序；取消可以抢占尚未发送的输入。控制消息使用独立或可优先调度的通道，不能被大量 stdin、stdout 或缓慢的输出消费者无限阻塞。缓冲区有界，背压作用于数据流。

exit 只表示主命令已经结束。completed 在后代清理和输出收尾后发送；正常路径应保证输出全部交付。超过收尾期限仍无法排空时，明确记录输出截断和清理状态，不能报告完整成功。

stdin EOF、连接断开、主命令退出和执行清理完成分别建模。PTY 会话的 stdin_eof 仅表示不再有输入，不伪造终端 EOF。

## 宿主终端与信号管理

CLI 的终端管理器统一保存、修改和恢复宿主 termios。SSH 和 libkrun 适配层必须有明确的终端所有权约定，避免双方同时进入 raw 模式或相互恢复旧状态。

guest 执行通道和 PTY 就绪后才进入 raw 模式。初始窗口设置完成后接通本地输入；在此之前保留正常宿主终端行为，使启动阶段的 Ctrl-C 可以取消启动。

退出顺序为停止本地输入读取、完成有界输出收尾、恢复宿主终端、输出必要诊断、返回主入口。深层库不能调用 os.Exit；主入口只在清理完成后决定进程退出状态。

宿主 SIGTSTP 先恢复 termios，再按系统作业控制挂起宿主进程，不映射为 guest Ctrl-Z。SIGCONT 后重新检查前台进程组；只有重新取得终端时才恢复 raw 模式并同步窗口。在后台不得继续读取或修改终端。挂起携带 VMM 的宿主进程也会暂停其 host 服务，因此它不作为后台持久运行接口。

信号在 CLI 边界集中订阅和路由，退出时撤销订阅。只注册本方案处理的信号，不订阅所有信号；尊重启动时继承的忽略设置，例如 nohup 的 SIGHUP。Go 与 libkrun 的 cgo 信号处理兼容性需要单独验证。

SIGKILL 和 SIGSTOP 无法捕获；单进程方案不能保证在这两种情况下恢复终端或完成清理。崩溃后的残留资源按资源生命周期方案回收。[Go signal 规则](https://pkg.go.dev/os/signal)

## 断管与退出结果

下游提前关闭 stdout 或 stderr，例如将输出接入 head，应触发当前执行的有界取消。可执行文件入口忽略进程级 SIGPIPE，由输出写入边界根据实际 EPIPE 取消当前命令并返回 141；这一策略持续到终端恢复和最终诊断结束。CLI 与 guest 输出使用同一个写入包装器，即使另一输出流阻塞也能通知 CLI 立即退出。内部 socket 的 EPIPE 由对应操作处理，例如 readiness 探测重试，不能被当成用户输出断管；进程级 SIGPIPE 无法识别出错的文件描述符，不再进入取消信号路由。取消不得依赖继续向已关闭的输出写入诊断。run 退出后由独立进程清理自己的 VM 资源，exec 保留 VM。[Go 的 SIGPIPE 行为](https://pkg.go.dev/os/signal#hdr-SIGPIPE)

结果记录分别保存命令退出状态、宿主取消原因、VM 清理状态和输出是否完整，避免一个 error 字符串覆盖所有信息。

| 结果 | CLI 目标行为 |
| --- | --- |
| guest 正常退出且收尾成功 | 保留 guest 退出码 |
| guest 被信号终止 | 保留信号名称，shell 退出码按 Linux 信号编号映射为 128 加信号编号 |
| 宿主取消先于命令完成被接受 | 以原始宿主信号决定取消退出码，同时记录最终 guest 结果 |
| 参数错误、启动失败、传输失败或清理失败 | 非零；使用类型化错误区分原因，具体保留码由 CLI 退出码规范统一确定 |
| 连接断开后无法取得命令结果 | 结果未知，不推断为 0 |

宿主取消导致 guest 的 trap 返回 0，仍不能把已经接受的宿主取消改写为成功。没有宿主取消的交互 Ctrl-C 则完全遵循 guest 结果；guest shell 可以处理按键后继续运行。

没有更早接受的宿主取消时，guest 已有非零结果则保留该值，同时报告清理错误；guest 为 0 而清理失败时，CLI 必须返回非零。错误类型与原始结果保存在运行记录中，因为数字退出码本身不能无歧义区分所有来源。

## 实施顺序

1. 引入类型化执行结果、集中信号路由和可测试的取消状态机，先确定资源所有权。
2. 实现 guest-control 双向流、stdin EOF、控制消息和独立执行监督器；修复退出码与输出收尾。
3. 实现 guest PTY、窗口变化和统一终端管理；run、attach 共用执行会话。
4. 将 run 调整为启动 VM、等待 agent 就绪、执行命令、收集结果、关闭 VM。服务就绪必须包含执行所需的挂载和网络准备，不能仅检查 socket 存在。
5. 接入有界 VM 停止、断连清理和执行 cgroup，验证后端强制停止边界。

SSH 可以在迁移期保留为兼容入口，但支持矩阵必须明确。旧 guest 缺少信号或 PTY 能力时返回可理解的能力错误，不能静默退化为无法取消或忽略命令的执行方式。现有 --pty 的兼容映射和默认 auto 行为变化需写入发布说明。

## 验收场景

| 场景 | 预期结果 |
| --- | --- |
| guest shell 运行 sleep 后按 Ctrl-C | sleep 被中断，返回 guest shell，VM 继续运行 |
| guest shell 连续收到 Ctrl-C | 不触发宿主强制关闭 VM |
| guest 内 Ctrl-Z 后执行 fg | guest 作业停止和恢复，宿主不误挂起 |
| 宿主 SIGTSTP 后以前台方式继续 | 宿主终端先恢复，继续后重新同步 raw 模式和窗口 |
| 调整终端尺寸 | guest TUI 收到最新尺寸，重复 resize 不影响执行 |
| attach --tty=always -- top | 执行 top，不替换为 shell |
| 管道输入到非 PTY cat 后 EOF | 完整输出输入内容，命令正常结束 |
| 强制 PTY 的输入管道结束 | 不伪造 Ctrl-D，不将输入 EOF 当成整条连接断开 |
| kill -TERM attach 进程 | 本次执行及其后代清理，VM 和其他执行不受影响 |
| guest 忽略终止信号 | 到期升级强制清理，宿主按停止预算收尾 |
| 启动期间取消或 started 响应丢失 | 不遗留无归属执行，不因重试重复启动命令 |
| 输出接入 head 后下游退出 | 有界取消，停止本次工作负载，无持续输出进程残留 |
| 大量 stdout 与 stderr 同时输出并取消 | 控制消息及时送达；正常退出不截断，超时截断有明确记录 |
| 子进程脱离进程组或保留输出 FD | 执行 cgroup 清理覆盖后代，收尾不会无限等待 EOF |
| guest exit 42 或被信号终止 | CLI 和运行记录保留相应结果，不统一转换成 1 |
| guest 成功但 VM 停止失败 | CLI 非零，分别记录命令成功和清理失败 |
| 启动失败、断连和正常退出 | 所有可处理路径都恢复宿主终端回显和模式 |

测试分为无需 VM 的状态机与协议测试、使用本地 PTY 的终端测试，以及 macOS 和 Linux 上的 VM 黑盒测试。强制停止、cgo 信号协作和终端恢复必须在实际后端验证，不能仅依赖模拟实现。
