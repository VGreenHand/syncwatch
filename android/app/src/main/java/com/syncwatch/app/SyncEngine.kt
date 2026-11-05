package com.syncwatch.app

import android.os.Handler
import android.os.Looper
import android.util.Log
import okhttp3.*
import okio.ByteString
import org.json.JSONObject
import java.util.concurrent.TimeUnit
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
        fun onConnected(room: String)
        fun onJoinError(msg: String)
        fun onMembers(members: List<MemberInfo>)
        fun onNotice(text: String)     // 操作归因/系统提示
        fun onDisconnected()
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
    private fun suppress(ms: Long) { suppressUntil = System.currentTimeMillis() + ms }
    private fun suppressed() = System.currentTimeMillis() < suppressUntil

    private val hb = object : Runnable {
        override fun run() {
            if (closed) return
            send(Protocol.heartbeat(player.currentPos(), player.currentRate()))
            if (player.isPlaying() && !player.isBuffering()) {
                send(Protocol.memberState("playing"))
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
                reconnectAttempts = 0
                roomCode = m.optString("room")
                helloReceived.set(true)
                val state = m.optString("state", "paused")
                val pos = m.optDouble("position", 0.0)
                val rate = m.optDouble("rate", 1.0).toFloat()
                roomState = state
                suppress(1500)
                player.seekTo(pos)
                if (state == "playing") player.play(pos, rate) else player.pause(pos)
                listener.onConnected(roomCode)
                listener.onMembers(Protocol.parseMembers(m.optJSONArray("members")))
            }
            "sync" -> {
                val operator = m.optString("operator")
                val state = m.optString("state")
                val pos = m.optDouble("position", 0.0)
                val rate = m.optDouble("rate", 1.0).toFloat()
                val needSeek = Math.abs(player.currentPos() - pos) > SOFT_DRIFT
                roomState = state
                suppress(1500)
                if (needSeek) player.seekTo(pos)
                if (state == "playing") player.play(pos, rate) else player.pause(pos)
                listener.onNotice("$operator ${if (state == "playing") "播放了" else "暂停了"} @${fmt(pos)}")
            }
            "forceSeek" -> {
                val pos = m.optDouble("position", 0.0)
                suppress(1500)
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
        if (!helloReceived.get() || suppressed()) return
        if (roomState == "playing") return
        roomState = "playing"
        send(Protocol.play(player.currentPos(), player.currentRate()))
        listener.onNotice("你播放了 @${fmt(player.currentPos())}")
    }

    fun onLocalPause() {
        if (!helloReceived.get() || suppressed()) return
        if (roomState == "paused") return
        roomState = "paused"
        send(Protocol.pause(player.currentPos()))
        listener.onNotice("你暂停了 @${fmt(player.currentPos())}")
    }

    fun onLocalSeek(pos: Double) {
        if (!helloReceived.get() || suppressed()) return
        send(Protocol.seek(pos))
        listener.onNotice("你跳转到了 ${fmt(pos)}")
    }

    fun onBufferingChanged(buffering: Boolean) {
        if (!helloReceived.get()) return
        send(Protocol.memberState(if (buffering) "buffering" else if (player.isPlaying()) "playing" else "paused"))
    }

    fun sendFileInfo(name: String, duration: Double) {
        send(Protocol.fileInfo(name, duration))
    }

    fun send(text: String) {
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
