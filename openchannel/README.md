# openchannel — 明渠水力核算服务

一个长期挂机的无状态 HTTP 服务，只管两件事：

1. **明渠均匀流反算**：给定断面（矩形 / 梯形）、曼宁糙率 `n`、渠底纵坡 `S0`、流量 `Q`，
   反求正常水深 `y_n`，并返回断面平均流速、弗劳德数、流态、水力半径等。
2. **薄壁矩形堰泄流**：给定堰宽 `b`、堰上水头 `H`（可选流量系数 `Cd`），算过堰流量。

不涉及有压管网，不做流域产汇流，没有任何页面。全部物理量采用 SI 单位（m、m²、m³/s）。

## 设计要点：一套几何，处处自洽

矩形只是边坡 `m=0` 的梯形特例，**全服务只有一份断面几何函数**
（`internal/geometry`）：

| 量 | 公式 | 用在哪 |
|---|---|---|
| 过流面积 `A` | `(b + m·y)·y` | 曼宁迭代、流速、Fr |
| 湿周 `P` | `b + 2·y·sqrt(1+m²)` | 水力半径 |
| 水力半径 `R` | `A/P` | 曼宁迭代 |
| 水面宽 `T` | `b + 2·m·y` | 弗劳德数 / 流态判定 |

曼宁公式（SI 制）：

```
Q = (1/n) · A · R^(2/3) · sqrt(S0)
```

弗劳德数与判流态使用的面积、水面宽与曼宁迭代**来自同一组函数**：

```
Fr = V / sqrt(g · A / T)
```

已知流量求水深是单调问题（`y` 增大时 `A`、`P` 同时增大，`Q(y)` 严格单调递增），
求解器（`internal/manning`）用「倍区间夹根 + 对分法」，不依赖经验初值，保证收敛。

堰流公式（无侧收缩薄壁矩形堰，默认 `Cd=0.42`）：

```
Q = Cd · b · sqrt(2g) · H^(3/2)
```

> 堰上水头 `H` 是相对**堰顶**的测压管水头，正常水深 `y_n` 从**渠底**起算，
> 二者不是同一物理量。若在均匀流请求里附带 `weir_head`，服务只在 `advisory`
> 字段做文字提示，绝不参与、也绝不修改任何计算。

## 包结构

```
internal/
  physics/   物理常量 g（单一来源）
  geometry/  断面几何：面积 / 湿周 / 水面宽 / 水力半径（唯一一套）
  manning/   曼宁公式与正常水深迭代求根（不放进 main）
  regime/    弗劳德数、流态判定、临界水深（含矩形闭式解）
  weir/      薄壁矩形堰流量
  validate/  计算前的输入校验
  server/    net/http 路由、JSON 编解码
cmd/openchannel/  main：只做装配、启动、优雅关停
```

输入校验在计算之前拦截：糙率 `n` 非正、坡度 `S0` 非正、底宽 `b` 非正、
边坡为负、流量为负、堰上水头非正、堰宽非正、NaN/Inf 等一律返回 `400`。

## 接口

### `GET /healthz`

```json
{"status":"ok"}
```

### `POST /v1/uniform-flow`

请求：

```json
{
  "bottom_width": 2.0,
  "side_slope": 0.0,
  "roughness": 0.030,
  "bed_slope": 0.0009,
  "discharge": 1.2599210498948732,
  "weir_head": 0.3
}
```

`side_slope` 省略或为 `0` 即矩形断面；`weir_head` 可选，仅触发提示。

响应：

```json
{
  "normal_depth": 1.0,
  "velocity": 0.62996,
  "froude_number": 0.20113,
  "regime": "subcritical",
  "regime_label": "缓流",
  "hydraulic_radius": 0.5,
  "area": 2.0,
  "top_width": 2.0,
  "discharge": 1.25992,
  "advisory": "数值上 y_n > H。提示：……不能直接比较……"
}
```

`regime` 取值：`subcritical`（缓流，Fr<1）、`critical`（临界流）、`supercritical`（急流，Fr>1）。

### `POST /v1/weir-flow`

请求：

```json
{"weir_width": 2.0, "head": 0.25, "discharge_coeff": 0.42}
```

`discharge_coeff` 省略时取默认 `0.42`。

响应：

```json
{"discharge":0.46509,"weir_width":2,"head":0.25,"discharge_coeff":0.42}
```

## 预置手算回归算例

矩形渠 `b=2 m, n=0.030, S0=0.0009`，取 `y=1 m`：

```
A = 2·1 = 2 m²,  P = 2+2 = 4 m,  R = 0.5 m
Q = (1/0.030)·2·0.5^(2/3)·sqrt(0.0009)
  = 2·0.5^(2/3)
  = 1.2599210498948732 m³/s
```

因此以 `Q=1.2599210498948732` 反求，正常水深必须在 `1.0 m` 量级——
该例同时钉在 `manning` 包与 HTTP 层的回归测试里。

## 测试

重点回归（`go test ./...`）：

- **水深反代收回流量**：矩形/梯形多组工况，解出 `y_n` 再代回曼宁公式，
  相对误差 `< 1e-9`；流速同时满足连续方程 `V=Q/A`。
- **矩形临界水深闭式解** `y_c = (Q²/(g·b²))^(1/3)`：
  用它抽查通用临界水深迭代、临界条件 `Q²T = gA³` 与流态判定；
  `y>y_c` 判缓流、`y<y_c` 判急流。
- 坡度调大同一流量的正常水深严格下降。
- 堰流水头按 `H^(3/2)` 增长、随堰宽与系数线性。
- 全部非法输入在计算前返回错误；`weir_head` 只出提示、不改数值。

## 本地运行

```bash
go test ./...
go run ./cmd/openchannel            # 默认监听 :8080
PORT=9090 go run ./cmd/openchannel
```

## Docker（基于 golang:1.22，单容器）

```bash
docker build -t openchannel:latest .
docker run --rm -p 8080:8080 openchannel:latest
```

容器内非 root 运行，自带 `/healthz` 健康检查，启动后单容器即可对外应答。
