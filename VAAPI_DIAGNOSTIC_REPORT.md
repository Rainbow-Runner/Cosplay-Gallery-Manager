# VAAPI 故障诊断报告

日期：2026-10-02；修复记录更新于2026-10-03  
依据：`Check_VAAPI.md`  
结论范围：本机 Linux Mint 22.1、Intel UHD Graphics 630、Ubuntu 24.04/Mint 22.1 提供的 Intel media driver 24.1.0

## 结论摘要

本次排查最终确认了两个先后暴露、彼此独立的问题；均不是 4K 竖屏、HEVC 解码、P010、rotation、scaler mode、媒体损坏或存档 Range 读取导致的。

直接原因是主机安装了 Ubuntu 的自由版 `intel-media-va-driver`（Free-Kernel build）。该构建在 Coffee Lake 上无法创建真正改变尺寸的 VAAPI VideoProc/VPP 缩放管线；`scale_vaapi` 在 `vaCreateConfig`/processing pipeline config 阶段返回 VA status 12：

```text
Failed to create processing pipeline config: 12
(the requested VAProfile is not supported)
```

同一台机器、同一个 `/dev/dri/renderD128`、同一版 FFmpeg/libva、同一条命令，仅通过 `LIBVA_DRIVERS_PATH` 临时切换到 Ubuntu 的 `intel-media-va-driver-non-free 24.1.0+ds1-1` 后：

- 全部分辨率、NV12/P010 和 scaler mode 测试通过；
- 真实 `2160×3840` HEVC 视频的 VAAPI 解码、缩放、H.264 编码全 GPU 链通过；
- 原本失败的 QSV VPP 链也通过。

因此，驱动构建差异是最初VPP初始化失败的已确认根因。初次排查时完整功能驱动尚未安装到系统，仅解包到 `/tmp` 做对照；其后的正式安装与复测见本报告末尾“安装后复测”。安装完整驱动后，真实业务视频又在EOF排空阶段暴露FFmpeg 6.1.1固定VAAPI VPP输出池缺陷；该第二问题已经通过FFmpeg源码、debug日志、libva trace和上游修复提交交叉确认，详见“EOF问题的最终根因”。

## Environment（初次排查）

```text
Linux Mint version: 22.1 (Xia), Ubuntu noble base
Kernel: 6.8.0-136-generic x86_64
GPU: Intel CoffeeLake-S GT2 / UHD Graphics 630
GPU PCI ID: 8086:3e92
DRM driver: i915
VAAPI device: /dev/dri/renderD128
renderD128 mapping: PCI 0000:00:02.0, vendor 0x8086, device 0x3e92
libva: 2.20.0
Intel media driver: Intel iHD 24.1.0
Installed package: intel-media-va-driver 24.1.0+dfsg1-1ubuntu0.2
Installed build: Free-Kernel build
FFmpeg: 6.1.1-3ubuntu5
FFmpeg VAAPI/libdrm: enabled
vainfo: not installed
```

主机另有 NVIDIA GTX 1050 Mobile（`renderD129`），但本报告只诊断 Intel `renderD128`。

Ubuntu 已安装软件包的描述明确指出：自由版在 Coffee Lake 上仅提供有限功能，media shaders 只在 Ice Lake 及更新平台可用。Intel 官方也说明 Video Processing 会组合使用 VEBox/SFC 与 media-kernel shader，并将 Ubuntu/Debian 的 `intel-media-va-driver` 定义为功能受限的 Free-Kernel build，将 `intel-media-va-driver-non-free` 定义为 Full-Feature build：

- [Intel media-driver README](https://github.com/intel/media-driver/blob/master/README.md)
- [Intel media-driver feature documentation](https://github.com/intel/media-driver/blob/master/docs/media_features.md)
- [Intel issue #1662：自由版 VideoProc 失败、完整功能版成功](https://github.com/intel/media-driver/issues/1662)

## Source Video

问题样本来自未压缩（7z `Method=Copy`）存档中的 `1.mp4`：

```text
codec: HEVC
profile: Main
bit depth: 8-bit
pix_fmt: yuv420p
resolution: 2160×3840
frame rate: 30 fps
color range: tv
field order: progressive
rotation: 无 side data
HDR: 未标记 HDR transfer/primaries
member size: 147087720 bytes
packed size: 147087720 bytes
```

因此该样本不是 HEVC Main10/P010，也不是 `3840×2160 + rotation=90°`。

## 最小复现命令

以下命令不依赖真实视频、HEVC 解码器或编码器，在当前已安装的自由版驱动上稳定失败：

```bash
ffmpeg -hide_banner -loglevel verbose \
  -vaapi_device /dev/dri/renderD128 \
  -f lavfi -i testsrc2=size=64x64:rate=1 \
  -vf 'format=nv12,hwupload,scale_vaapi=w=32:h=32:format=nv12' \
  -frames:v 1 -an -f null -
```

第一条关键错误即为 processing pipeline config 创建失败，之后的 filter reinitialization 与 `Conversion failed` 只是级联结果。

如果将输出维持为 `64×64`，命令成功；这并未执行有效缩放，不能证明 VPP scaling 可用。

## Test Matrix：当前自由版驱动

| ID | 输入 | 格式 | 输出 | Mode | 结果 |
| --- | --- | --- | --- | --- | --- |
| R1 | 3840×2160 | NV12 | 1920×1080 | default | FAIL 251 |
| R2 | 2160×3840 | NV12 | 1080×1920 | default | FAIL 251 |
| R3 | 4096×2160 | NV12 | 2048×1080 | default | FAIL 251 |
| R4 | 2160×4096 | NV12 | 1080×2048 | default | FAIL 251 |
| R5 | 1920×3840 | NV12 | 960×1920 | default | FAIL 251 |
| H2160 | 2160×2160 | NV12 | 1080×1080 | default | FAIL 251 |
| H2304 | 2160×2304 | NV12 | 1080×1152 | default | FAIL 251 |
| H2560 | 2160×2560 | NV12 | 1080×1280 | default | FAIL 251 |
| H2880 | 2160×2880 | NV12 | 1080×1440 | default | FAIL 251 |
| H3072 | 2160×3072 | NV12 | 1080×1536 | default | FAIL 251 |
| H3200 | 2160×3200 | NV12 | 1080×1600 | default | FAIL 251 |
| H3456 | 2160×3456 | NV12 | 1080×1728 | default | FAIL 251 |
| H3600 | 2160×3600 | NV12 | 1080×1800 | default | FAIL 251 |
| H3840 | 2160×3840 | NV12 | 1080×1920 | default | FAIL 251 |
| F1 | 2160×3840 | NV12 | 1080×1920 | default | FAIL 251 |
| F2 | 2160×3840 | P010 | 1080×1920 | default | FAIL 251 |
| F3 | 2160×3840 | P010→NV12 | 1080×1920 | default | FAIL 251 |
| S1 | 2160×3840 | NV12 | 1080×1920 | fast | FAIL 251 |
| S2 | 2160×3840 | NV12 | 1080×1920 | default | FAIL 251 |
| S3 | 2160×3840 | NV12 | 1080×1920 | hq | FAIL 251 |
| D64 | 64×64 | NV12 | 32×32 | default | FAIL 251 |
| D480 | 854×480 | NV12 | 426×240 | default | FAIL 251 |
| D720 | 1280×720 | NV12 | 640×360 | default | FAIL 251 |
| D1080 | 1920×1080 | NV12 | 960×540 | default | FAIL 251 |
| D1080P | 1080×1920 | NV12 | 540×960 | default | FAIL 251 |
| D1440 | 2560×1440 | NV12 | 1280×720 | default | FAIL 251 |
| D1440P | 1440×2560 | NV12 | 720×1280 | default | FAIL 251 |
| D1920SQ | 1920×1920 | NV12 | 960×960 | default | FAIL 251 |
| D2048 | 2048×1080 | NV12 | 1024×540 | default | FAIL 251 |
| B1 | 2160×3840 | NV12 | 不缩放 | none | PASS |

所有 FAIL 的首个错误相同，不存在可供二分搜索的高度边界。即使 `64×64→32×32` 也失败，因此 Case A“竖屏高度限制”和 Case B“P010 限制”均不成立。

## A/B 对照：临时完整功能驱动

下载但未安装：

```text
intel-media-va-driver-non-free 24.1.0+ds1-1
```

临时设置 `LIBVA_DRIVERS_PATH` 指向 `/tmp` 解包目录后，上表所有 R、H、F、S、D 与 B 项全部 PASS，包含：

- `3840×2160` 与 `2160×3840`；
- `2160×4096`；
- NV12、P010、P010→NV12；
- fast、default、hq；
- `64×64→32×32`；
- 真实样本的 HEVC VAAPI decode → `scale_vaapi` → H.264 VAAPI encode。

真实样本处理 30 帧的观测速度约为 2.11× realtime。

## Important Findings

1. HEVC VAAPI 解码 30 帧成功，约 2.48× realtime。
2. 不使用 VPP 缩放时，真实视频 VAAPI 解码 → H.264 VAAPI 编码成功，约 1.53× realtime。
3. 移除编码器、只执行 `scale_vaapi` 到 null 仍失败，编码器不是故障点。
4. 使用 `testsrc2 + NV12 + hwupload` 仍失败，HEVC decoder 不是必要故障条件。
5. NV12 与 P010 都失败，问题不属于 10-bit/P010 特例。
6. fast/default/hq 都失败，问题不属于某一种 scaler algorithm。
7. 横屏与竖屏、低分辨率与 4K 都失败，问题不属于方向或尺寸边界。
8. 临时完整功能驱动使所有测试通过，确认是已安装驱动的构建功能限制。
9. QSV 在自由版驱动下于 `Error querying VPP params: unsupported (-3)` 失败；完整功能驱动下成功，说明 QSV 不能绕过缺失的底层 VPP 功能。

## Failure Layer

| 层级 | 状态 | 证据 |
| --- | --- | --- |
| 存档读取/Range | 排除 | 样本可提取；失败也可由 testsrc2 复现 |
| HEVC demux/decode | PASS | 真实视频 VAAPI decode 30 帧成功 |
| VAAPI device/surface | PASS | `hwupload` 与解码 surface 成功 |
| VPP pipeline configuration | **FAIL** | 第一条错误是 processing pipeline config / VAProfile unsupported |
| VPP execution | 未进入 | 配置阶段已经失败 |
| H.264 VAAPI encode | PASS | 不缩放和 CPU 缩放混合链均成功 |

## Root Cause

### Confirmed

- 故障由主机当前安装的 Free-Kernel `intel-media-va-driver` 构建触发。
- 换为相同主版本的 Full-Feature `intel-media-va-driver-non-free` 后，同一硬件与命令全部成功。
- 当前失败点是 VAAPI VideoProc/VPP pipeline configuration，而非执行后期。

### Highly likely

- Coffee Lake 上自由版构建缺失该 VPP 路径所需的 legacy media-kernel/shader 功能。该判断同时得到 Ubuntu 包描述、Intel 构建说明以及 Intel issue #1662 的支持。

### Possible but not required for repair

- 具体缩放请求最终选择的是 SFC、EU shader，还是驱动在二者间的 fallback，在未启用 libva trace/驱动内部调试的情况下不能进一步断言；这不影响“驱动构建是根因”的 A/B 结论。

### Excluded

- Intel UHD 630 完全不支持 VAAPI；
- HEVC 4K 解码失败；
- 竖屏高度 3840 的硬件上限；
- Main10/P010 特有故障；
- rotation metadata；
- HDR；
- `fast/default/hq` 中某个模式特有故障；
- H.264 VAAPI 编码失败；
- 7z 存档或 Range bridge 导致失败。

## CGM 暴露出的诊断缺口

### 1. VAAPI 自检是假阳性

当前自检使用 `64×64 → 64×64`：

```text
format=nv12,hwupload,scale_vaapi=64:64
```

该测试不改变尺寸，驱动可以旁路真正的缩放处理，所以状态被错误标为 `AVAILABLE`。自检至少应使用 `64×64 → 32×32`，并保留真实 decode → scale → encode 测试。

### 2. 运行时没有触发软件回退

当前错误分类没有识别：

```text
Failed to create processing pipeline config
the requested VAProfile is not supported
```

因此 CGM 将其记为普通 `GENERATION_FAILED`，没有包装为硬件执行失败，也没有触发一次性软件 fallback 或硬件熔断；HLS 失败后，完整 MP4 任务又进行了重复尝试。

### 3. `AVAILABLE` 的含义需要收紧

设备可打开、解码器可用、编码器可用，不等于 VPP scaling 可用。诊断状态应分别反映 decode、encode、scale 三项实际能力。

## Workarounds

| 方案 | 全 GPU | CPU copy | 实测结果 | 性能/稳定性 | 开发复杂度 |
| --- | --- | --- | --- | --- | --- |
| 安装 Full-Feature `intel-media-va-driver-non-free` | 是 | 否 | **PASS，推荐** | 真实样本约 2.11× realtime；完整矩阵通过 | 主机软件包变更；CGM 无需改转码图 |
| VAAPI decode → CPU scale → VAAPI encode | 否 | 有，下载+上传 | PASS | 真实样本约 0.56× realtime，低于实时 | 中等；需新增混合 filter graph |
| 不缩放，VAAPI decode → VAAPI encode | 是 | 否 | PASS | 约 1.53× realtime；输出仍是 2160×3840 | 低，但输出过大且浏览器兼容/带宽风险高 |
| QSV decode → scale_qsv → QSV encode | 是 | 否 | 自由版 FAIL；完整驱动 PASS | 完整驱动约 2.23× realtime | 不能替代正确的底层驱动 |
| NVENC | 是 | 取决于输入链 | 此前主机自检 AVAILABLE | 可绕过 Intel VPP | CGM 已支持，需切换后端 |
| 全软件转码 | 否 | 不适用 | 可作为安全 fallback | CPU 占用最高 | CGM 已具备软件路径 |

## Recommended Pipeline

主机层首选：

```text
intel-media-va-driver-non-free
  + HEVC VAAPI decode
  + scale_vaapi
  + H.264 VAAPI encode
```

CGM 层仍应补齐防御性闭环：

1. 将 VAAPI smoke test 改成真实尺寸变化；
2. 将上述两个明确错误标记识别为硬件/VPP 故障；
3. 触发一次性软件 fallback，并对同一后端进入运行时熔断；
4. 在管理页面明确展示 decode/scale/encode 分项结果；
5. 即便主机修复驱动，也保留上述逻辑，以适配其他机器和驱动组合。

正式切换主机驱动前应由用户确认许可/授权接受程度。建议操作是用发行版软件包替换，而不是把 `/tmp` 解包目录作为长期运行依赖。切换后再用真实业务视频完成 HLS 首段、完整 MP4、seek 与缓存复用验收。

## 安装后复测（2026-10-02）

主机已正式完成软件包替换：

```text
intel-media-va-driver: not installed
intel-media-va-driver-non-free: 24.1.0+ds1-1 installed
```

未使用 `LIBVA_DRIVERS_PATH` 覆盖，直接调用系统 `iHD_drv_video.so` 的结果如下：

### 驱动能力验收

- 最小真实缩放 `64×64 → 32×32`：PASS；
- 4K 竖屏缩放 `2160×3840 → 1080×1920`：PASS；
- 完整分辨率、格式和 scaler mode 矩阵：30/30 PASS；
- NV12、P010、P010→NV12：PASS；
- fast、default、hq：PASS。

这确认系统正式驱动已经修复 Free-Kernel build 导致的 VideoProc/VPP 不可用问题。

### CGM 外部集成测试

以下项目自带外部测试全部 PASS：

```text
TestCompleteVideoVAAPIExternal
TestVAAPIProbeExternal
TestProgressiveVideoVAAPIExternal
TestVAAPIProgressiveSessionEndToEnd/directory-complete
TestVAAPIProgressiveSessionEndToEnd/tar-range-seek
TestVAAPIProgressiveSessionEndToEnd/sevenzip-range-seek
```

覆盖完整 MP4、渐进 HLS、缓存完成、目录来源、TAR Range seek 与 7z Range seek。

### 真实业务文件的剩余兼容问题

对问题存档中的完整 `1.mp4`（2550 帧）进行正式参数复测时发现第二个、独立于驱动构建的问题：

| 链路 | 结果 |
| --- | --- |
| 软件解码 → null | PASS，2550/2550 |
| VAAPI 解码 → null | PASS，2550/2550 |
| VAAPI 解码 → `scale_vaapi` → null | PASS，2550/2550 |
| VAAPI 解码 → H.264 VAAPI 编码，不缩放 | PASS，2550/2550 |
| VAAPI 解码 → `scale_vaapi` → H.264 VAAPI 编码 | **FAIL，2542/2550** |
| VAAPI 解码 → CPU scale → H.264 VAAPI 编码 | PASS，尾部 150/150 |
| QSV decode → `scale_qsv` → QSV encode | PASS，无错误退出 |

组合 VAAPI 链只在 EOF/drain 阶段失败，FFmpeg 返回：

```text
Error while filtering: Cannot allocate memory
Failed to inject frame into filter network: Cannot allocate memory
```

现象稳定缺少最后 8 个 HEVC 重排序帧。`-extra_hw_frames 8` 与将 `h264_vaapi async_depth` 从 2 降为 1 均不能修复。VPP-only 与 encoder-only 均成功，故该问题位于 VPP 输出 surface 直接交给 H.264 VAAPI encoder 的组合 drain/协商路径，而不是解码器、单独 VPP 或单独编码器。

### EOF 问题的最终根因

该问题已经进一步定位为 **FFmpeg 6.1.1 的 VAAPI filter 固定大小输出 frame pool 缺陷**，不是 Intel iHD 驱动再次失败，也不是媒体文件损坏。

FFmpeg 6.1.1 的 `libavfilter/vaapi_vpp.c` 明确使用固定池：

```c
output_frames->initial_pool_size = 4;
err = ff_filter_init_hw_frames(avctx, outlink, 10);
```

FFmpeg 主程序和 H.264 VAAPI encoder 会在下游队列中继续持有 VPP 输出 surface。正常持续处理时，编码器会逐步同步并释放 surface，因此暂时不暴露；到输入 EOF 时，HEVC decoder 一次排出最后的重排序帧，短时间内需要的 VPP 输出 surface 数超过固定池，`scale_vaapi` 从 FFmpeg 自己的 buffer pool 取不到下一张 surface，返回 `AVERROR(ENOMEM)`。

本机 debug 日志显示：

- VPP 输出反复使用固定集合 `0x1b` 至 `0x24`；
- 输入 EOF 后，decoder 开始 drain；
- 7 张尾帧先占用 `0x20` 至 `0x1b` 等剩余 surface；
- 第 8 张申请不到 VPP 输出 surface，filter 返回 `Cannot allocate memory`；
- 因 filter graph 整体失败，已经进入队列但尚未编码的 7 张尾帧也被丢弃，最终正好少 8 帧。

libva trace 同时证明所有实际提交给驱动的操作均成功：

```text
vaBeginPicture   VA_STATUS_SUCCESS
vaRenderPicture  VA_STATUS_SUCCESS
vaEndPicture     VA_STATUS_SUCCESS
vaSyncBuffer     VA_STATUS_SUCCESS
```

失败前没有新的 `vaCreateSurfaces` 或其他 VA 调用返回错误；ENOMEM 在 FFmpeg 固定池层产生，随后才开始正常销毁 VA context/surface。这排除了 Intel 驱动或 GPU 显存真的耗尽。

FFmpeg 上游针对同一故障在提交 `16616a3d1be07d1b20268df1bd5727bb4ca33c92` 中将 VAAPI filter 输出改为动态 frame pool。上游提交说明给出的复现链和错误与本机一致：`scale_vaapi → VAAPI encoder`，随后 `Error while filtering: Cannot allocate memory`。

FFmpeg 7.1 对应源码为：

```c
if (CONFIG_VAAPI_1)
    output_frames->initial_pool_size = 0;
else
    output_frames->initial_pool_size = 4;
```

在当前 libva 2.20 环境中会选择 `initial_pool_size = 0`，按需动态创建 surface，不再受固定池容量限制。

上游依据：

- [FFmpeg 提交记录：VAAPI VPP 使用动态 frame pool](https://ffmpeg.org/pipermail/ffmpeg-cvslog/2024-April/142627.html)
- [FFmpeg-devel：固定池、下游队列与 ENOMEM 的完整讨论](https://ffmpeg.org/pipermail/ffmpeg-devel/2024-February/322169.html)
- [FFmpeg 提交：为内部队列正确设置 extra_hw_frames](https://ffmpeg.org/pipermail/ffmpeg-cvslog/2024-March/141494.html)
- [FFmpeg 7.1 发布信息](https://ffmpeg.org/?pubDate=20260426)

`-extra_hw_frames 8` 没有解决本机问题，是因为该命令行选项扩充的是 decoder hardware-frame 需求，而本次耗尽的是 `scale_vaapi` 的输出池；FFmpeg 6.1.1 的 `scale_vaapi` 没有提供可以从 CGM 命令行安全扩大这一固定输出池的等价参数。

版本边界方面，FFmpeg 7.0 分支在 2024-03-25 切出，而上述 VPP 动态池提交日期为 2024-03-26，因此不能把“升级到任意 7.0”作为可靠修复。应使用 FFmpeg 7.1 或更新版本，或者在经过维护与测试的 FFmpeg 6.1.1 构建中明确回移该上游提交。

该问题不会推翻完整驱动已经修复 VPP 初始化的结论，但意味着使用系统 FFmpeg 6.1.1 时，当前真实业务视频的 VAAPI 播放链尚不能判定完全验收。处理优先级建议为：

1. 首选验证并采用 FFmpeg 7.1+，保持 VAAPI 全 GPU 链；
2. 无法升级 FFmpeg 时，对 ENOMEM 触发混合 CPU scale + VAAPI encode fallback；
3. 后续正式实现并验证 QSV 执行后端；
4. 使用已验收的 NVENC 或软件 fallback；
5. 无论 FFmpeg 是否升级，都应保留运行时错误分类、fallback 与熔断。

## CGM修复实施（2026-10-03）

- VAAPI启动探测改为真实尺寸变化，并以24帧带B帧HEVC完整排空到EOF，不再用单帧或同尺寸滤镜形成假阳性。
- 完整`scale_vaapi`链失败时继续验证`hwdownload → CPU scale → hwupload`；本机FFmpeg 6.1.1现在报告兼容混合管线，而非错误宣称完整设备滤镜链可用。
- 规划器v4让HLS和完整MP4按探测结果冻结完整或混合VAAPI策略，并把二者放入不同缓存Profile。
- `Cannot allocate memory`的filter drain模式与VPP／VAProfile配置失败分别映射为稳定硬件技术错误码。允许回退时，完整VAAPI只尝试一次兼容VAAPI；兼容链仍失败才转软件并熔断后端。
- 本机定向门禁已通过完整MP4、渐进HLS，以及目录、TAR、Copy 7z端到端来源。修复已提交为`34c5625a43e4cabebadac6b124a1000597c93c9a`并完成本机部署；正式程序SHA-256为`0edbbbb24774f3d72539cfd9067ad5db15ac2f3954fe0e42bfe287c0624a863b`，About精确对应提交。

## Artifacts

本次未修改系统驱动，也未修改业务媒体。临时诊断材料位于：

```text
/tmp/cgm-vaapi-debug/logs/results-free-driver.csv
/tmp/cgm-vaapi-debug/logs/results-nonfree-driver.csv
/tmp/cgm-vaapi-debug/logs/summary-free-driver.txt
/tmp/cgm-vaapi-debug/logs/summary-nonfree-driver.txt
/tmp/cgm-vaapi-debug/logs/baseline/
/tmp/cgm-vaapi-debug/logs/workaround/
/tmp/cgm-vaapi-debug/logs/system/problem-ffprobe.json
/tmp/cgm-vaapi-retest/results-installed-nonfree.csv
/tmp/cgm-vaapi-retest/summary-installed-nonfree.txt
/tmp/cgm-vaapi-retest/
```

`/tmp` 中另有为测试临时提取的视频样本和解包的完整功能驱动；重启后可自然丢失，也可在诊断验收完成后清理。
