package com.syncwatch.app

import android.os.Handler
import android.os.Looper
import android.util.Log
import okhttp3.*
import okio.ByteString
import org.json.JSONObject
import java.util.concurrent.TimeUnit
import java.util.Locale
import java.util.concurrent.atomic.AtomicBoolean

/**
 * 同步引擎：WebSocket 信令 <-> ExoPlayer 桥接
 */
class SyncEngine(
    private val server: String,          // host:port
    private val playerName: String,
    private val player: PlayerAdapter,
    private val listener: Listener
) {
    interface PlayerAdapter {
        fun play(posSec: Double, rate: Float)      // 应用远端 play
        fun pause(posSec: Double)                  // 应用远端 pause
        fun seekTo(posSec: Double)                 // 应用远端 seek
        fun currentPos(): Double
        fun currentRate(): Float
        fun isPlaying(): Boolean
        fun isBuffering(): Boolean
    }

    interface Listener {
        // created=true 表示本次新建了房间（输入的房间码不存在时也会新建）
        fun onConnected(room: String, created: Boolean)
        fun onJoinError(msg: String)
        fun onMembers(members: List<MemberInfo>)
        fun onNotice(text: String)     // 操作归因/系统提示
        fun onDisconnected()

        // 下面是纯 UI 状态通知：不改变协议，也不改变任何同步判定。
        fun onReconnecting(attempt: Int) {}
        fun onRemoteSeek(position: Double) {}
    }

    companion object {
        private const val TAG = "SyncEngine"
        private const val HB_INTERVAL = 5000L
        private const val RECONNECT_DELAY = 5000L
        private const val RECONNECT_MAX = 30
        private const val SOFT_DRIFT = 0.3
    }

    private val client = OkHttpClient.Builder()
        .pingInterval(20, TimeUnit.SECONDS)
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .build()

    private var ws: WebSocket? = null
    private val main = Handler(Looper.getMainLooper())
    private val helloReceived = AtomicBoolean(false)

    // 服务器最近一次告知的房间状态（playing/paused）。
    // 本地事件与之一致 → 说明是"应用远端状态"引起的回声或播放器初始化事件，不予上报，
    // 否则加入一个正在播放的房间会把全房间拉回 0 秒（Windows 端同一处修复）。
    @Volatile private var roomState: String = "paused"
    private var roomCode = ""
    private var password = ""
    private var reconnectAttempts = 0
    private var closed = false

    // 本地操作抑制窗口（应用远端指令时不回发）
    @Volatile private var suppressUntil = 0L
    // 上次上报过的成员状态，避免每 5 秒重复发
    private var lastSentMemberState = ""
    private fun suppress(ms: Long) { suppressUntil = System.currentTimeMillis() + ms }
    private fun suppressed() = System.currentTimeMillis() < suppressUntil

    private val hb = object : Runnable {
        override fun run() {
            if (closed) return
            send(Protocol.heartbeat(player.currentPos(), player.currentRate()))
            val st = when {
                player.isBuffering() -> "buffering"
                player.isPlaying() -> "playing"
                else -> "paused"
            }
            if (st != lastSentMemberState) {
                lastSentMemberState = st
                send(Protocol.memberState(st))
            }
            main.postDelayed(this, HB_INTERVAL)
        }
    }

    fun connect(room: String?, pass: String) {
        password = pass
        val url = "ws://$server/ws"
        Log.i(TAG, "connect $url room=$room")
        val req = Request.Builder().url(url).build()
        ws = client.newWebSocket(req, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                webSocket.send(Protocol.join(room, playerName, password))
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                main.post { handleMessage(text) }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                Log.w(TAG, "ws failure: ${t.message}")
                handleDisconnect()
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                handleDisconnect()
            }
        })
    }

    private fun handleDisconnect() {
        if (closed) return
        main.post {
            helloReceived.set(false)
            listener.onDisconnected()
            if (reconnectAttempts < RECONNECT_MAX) {
                reconnectAttempts++
                listener.onReconnecting(reconnectAttempts)
                listener.onNotice("连接断开，第 $reconnectAttempts 次重连…")
                main.postDelayed({
                    if (!closed) connect(roomCode.ifBlank { null }, password)
                }, RECONNECT_DELAY)
            }
        }
    }

    private fun handleMessage(text: String) {
        val m = try { JSONObject(text) } catch (e: Exception) { return }
        when (m.optString("type")) {
            "hello" -> {
                Log.i(TAG, "recv hello room=${m.optString("room")} state=${m.optString("state")} pos=${m.optDouble("position")}")
                reconnectAttempts = 0
                roomCode = m.optString("room")
                helloReceived.set(true)
                val state = m.optString("state", "paused")
                val pos = m.optDouble("position", 0.0)
                val rate = m.optDouble("rate", 1.0).toFloat()
                roomState = state
                suppress(600)
                player.seekTo(pos)
                if (state == "playing") player.play(pos, rate) else player.pause(pos)
                listener.onConnected(roomCode, m.optBoolean("created", false))
                listener.onMembers(Protocol.parseMembers(m.optJSONArray("members")))
            }
            "sync" -> {
                Log.i(TAG, "recv sync state=${m.optString("state")} pos=${m.optDouble("position")} op=${m.optString("operator")} local=$roomState")
                val operator = m.optString("operator")
                val state = m.optString("state")
                var pos = m.optDouble("position", 0.0)
                val rate = m.optDouble("rate", 1.0).toFloat()
                val stateChanged = roomState != state
                val local = player.currentPos()
                // 位置防护：0 或非法值只在"本来就在片头"时才相信。
                // 对端偶尔会把读不到 time-pos 的 0 报上来，跟着 seek(0) 就是"跳回片头"。
                if (pos.isNaN() || pos < 0 || (pos <= 0.05 && local > 5.0)) {
                    pos = local
                }
                // 状态变化时严一点（人对播放/暂停敏感），纯位置同步时松一点（避免频繁 seek 卡顿）
                val threshold = if (stateChanged) 0.5 else 1.0
                val needSeek = Math.abs(local - pos) > threshold
                // 房间状态没变但收到回执 = 其它成员调了进度或倍速。通知 UI 取消本地未提交的
                // 滑动预览，否则用户松手时会把朋友刚调好的进度覆盖回去。
                if (!stateChanged) listener.onRemoteSeek(pos)
                roomState = state
                suppress(400)
                if (needSeek) player.seekTo(pos)
                if (state == "playing") player.play(pos, rate) else player.pause(pos)
                // 服务端 seek 防抖定时器会把最终位置再广播一次（operator 仍是发起者）。
                // 发起端已经显示过"你跳转到了"，这里不能再说成"某某调整了播放"。
                if (operator != playerName) {
                    listener.onNotice(
                        when {
                            stateChanged -> "$operator ${if (state == "playing") "播放了" else "暂停了"} @${fmt(pos)}"
                            needSeek -> "$operator 调整了进度到 ${fmt(pos)}"
                            else -> "$operator 调整了播放"
                        }
                    )
                }
            }
            "forceSeek" -> {
                var pos = m.optDouble("position", 0.0)
                // 纠偏也不能盲从：明显不合理的 0 视为服务端脏数据
                if (pos.isNaN() || pos < 0 || (pos <= 0.05 && player.currentPos() > 5.0)) {
                    return
                }
                listener.onRemoteSeek(pos)
                suppress(400)
                player.seekTo(pos)
                listener.onNotice("服务器纠偏 → ${fmt(pos)}")
            }
            "members" -> listener.onMembers(Protocol.parseMembers(m.optJSONArray("members")))
            "join" -> listener.onNotice("${m.optString("name")} 加入了房间")
            "leave" -> listener.onNotice("${m.optString("name")} 离开了房间")
            "joinError" -> {
                listener.onJoinError(m.optString("error", "加入失败"))
                close()
            }
            "error" -> listener.onNotice("服务器: ${m.optString("error")}")
        }
    }

    // ---------- 本地操作上报（由 PlayerAdapter 回调触发） ----------

    fun onLocalPlay() {
        Log.i(TAG, "onLocalPlay hello=${helloReceived.get()} suppress=${suppressed()} roomState=$roomState")
        if (!helloReceived.get() || suppressed()) return
        if (roomState == "playing") return
        roomState = "playing"
        send(Protocol.play(player.currentPos(), player.currentRate()))
        sendMemberState("playing")
        listener.onNotice("你播放了 @${fmt(player.currentPos())}")
    }

    fun onLocalPause() {
        Log.i(TAG, "onLocalPause hello=${helloReceived.get()} suppress=${suppressed()} roomState=$roomState buffering=${player.isBuffering()}")
        if (!helloReceived.get() || suppressed()) return
        // 缓冲/卡顿不是暂停，别把全房间停住
        if (player.isBuffering()) return
        if (roomState == "paused") return
        roomState = "paused"
        send(Protocol.pause(player.currentPos()))
        sendMemberState("paused")
        listener.onNotice("你暂停了 @${fmt(player.currentPos())}")
    }

    fun onLocalSeek(pos: Double) {
        Log.i(TAG, "onLocalSeek pos=$pos hello=${helloReceived.get()} suppress=${suppressed()}")
        if (!helloReceived.get() || suppressed()) return
        send(Protocol.seek(pos))
        listener.onNotice("你跳转到了 ${fmt(pos)}")
    }

    /**
     * 倍速没有单独协议消息，沿用 play 的 rate 字段：
     * 只在房间处于播放中时上报，避免暂停时选倍速把全房间意外拉起。
     * 暂停时设置的倍速会留在本地，下一次 play 时随 rate 一起同步。
     */
    fun onLocalRateChanged(rate: Float) {
        Log.i(TAG, "onLocalRateChanged rate=$rate hello=${helloReceived.get()} roomState=$roomState")
        if (!helloReceived.get() || suppressed()) return
        if (roomState != "playing") return
        send(Protocol.play(player.currentPos(), rate))
        listener.onNotice("你调整了 ${String.format(Locale.CHINA, "%.2f", rate)}× 倍速")
    }

    fun onBufferingChanged(buffering: Boolean) {
        if (!helloReceived.get()) return
        send(Protocol.memberState(if (buffering) "buffering" else if (player.isPlaying()) "playing" else "paused"))
    }

    fun sendFileInfo(name: String, duration: Double) {
        send(Protocol.fileInfo(name, duration))
    }

    /** 立刻同步成员状态（有变化才发），避免界面要等下一次心跳才更新 */
    private fun sendMemberState(st: String) {
        if (st == lastSentMemberState) return
        lastSentMemberState = st
        send(Protocol.memberState(st))
    }

    fun send(text: String) {
        Log.i(TAG, "send ${text.take(140)}")
        ws?.send(text)
    }

    private fun fmt(d: Double) = String.format("%d:%02d", (d / 60).toInt(), (d % 60).toInt())

    fun close() {
        closed = true
        main.removeCallbacks(hb)
        send(Protocol.leave())
        ws?.close(1000, "bye")
        client.dispatcher.executorService.shutdown()
    }

    fun startHeartbeat() {
        main.postDelayed(hb, HB_INTERVAL)
    }
}
