package com.syncwatch.app

import android.content.Context
import android.util.Log
import io.livekit.android.ConnectOptions
import io.livekit.android.LiveKit
import io.livekit.android.events.RoomEvent
import io.livekit.android.events.collect
import io.livekit.android.room.Room
import io.livekit.android.room.track.AudioTrack
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * 最小 LiveKit 语音客户端：连接语音房、开关麦克风、接收远端音轨事件。
 * 不参与 SyncWatch 的 WebSocket 信令；生命周期与 UI 接入见 T3 后续步骤。
 */
class VoiceClient(context: Context, private val diagnostics: DiagnosticLog) {
    interface Listener {
        fun onVoiceConnected()
        fun onVoiceReconnecting()
        fun onVoiceReconnected()
        fun onVoiceDisconnected(reason: String)
        fun onRemoteAudioSubscribed(name: String)
        fun onRemoteAudioUnsubscribed(name: String)
        fun onVoiceError(message: String)
        fun onVoiceSubscriptionError(message: String)
    }

    companion object {
        private const val TAG = "VoiceClient"
    }

    private val appContext = context.applicationContext
    private var scope: CoroutineScope? = null
    private var room: Room? = null
    private var listener: Listener? = null

    fun connect(url: String, token: String, listener: Listener) {
        disconnect()
        this.listener = listener
        val s = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
        scope = s
        val r = LiveKit.create(appContext)
        room = r
        Log.i(TAG, "connect url=$url tokenLen=${token.length}")
        diagnostics.event("voice", "connect url=$url tokenLen=${token.length}")

        s.launch {
            r.events.collect { event ->
                if (scope !== s) return@collect
                when (event) {
                    is RoomEvent.Connected -> {
                        Log.i(TAG, "connected")
                        diagnostics.event("voice", "connected")
                        listener.onVoiceConnected()
                    }
                    is RoomEvent.Reconnecting -> {
                        diagnostics.event("voice", "reconnecting")
                        listener.onVoiceReconnecting()
                    }
                    is RoomEvent.Reconnected -> {
                        diagnostics.event("voice", "reconnected")
                        listener.onVoiceReconnected()
                    }
                    is RoomEvent.Disconnected -> {
                        Log.i(TAG, "disconnected reason=${event.reason} error=${event.error?.message}")
                        diagnostics.event(
                            "voice",
                            "disconnected reason=${event.reason} error=${event.error?.message}"
                        )
                        listener.onVoiceDisconnected(event.reason.toString())
                    }
                    is RoomEvent.TrackSubscribed -> if (event.track is AudioTrack) {
                        val name = event.participant.name ?: event.participant.identity?.value ?: "对方"
                        Log.i(TAG, "remote audio subscribed from $name")
                        diagnostics.event("voice", "remote audio subscribed from $name")
                        listener.onRemoteAudioSubscribed(name)
                    }
                    is RoomEvent.TrackUnsubscribed -> if (event.track is AudioTrack) {
                        val name = event.participant.name ?: event.participant.identity?.value ?: "对方"
                        diagnostics.event("voice", "remote audio unsubscribed from $name")
                        listener.onRemoteAudioUnsubscribed(name)
                    }
                    is RoomEvent.TrackSubscriptionFailed -> {
                        diagnostics.event(
                            "voice",
                            "track subscription failed: ${event.exception.message ?: "未知错误"}"
                        )
                        listener.onVoiceSubscriptionError(
                            "语音订阅失败：${event.exception.message ?: "未知错误"}"
                        )
                    }
                    is RoomEvent.FailedToConnect -> {
                        diagnostics.event(
                            "voice",
                            "connect failed event: ${event.error.message ?: "语音连接失败"}"
                        )
                        listener.onVoiceError(event.error.message ?: "语音连接失败")
                    }
                    else -> Unit
                }
            }
        }

        s.launch {
            try {
                r.connect(
                    url,
                    token,
                    ConnectOptions(autoSubscribe = true, audio = false, video = false)
                )
            } catch (e: CancellationException) {
                // 主动断开/重连时忽略旧连接的取消
            } catch (e: Exception) {
                Log.w(TAG, "connect failed: ${e.message}")
                diagnostics.event("voice", "connect failed: ${e.message ?: "unknown"}")
                if (scope === s) listener.onVoiceError(e.message ?: "语音连接失败")
            }
        }
    }

    fun setMicrophoneEnabled(enabled: Boolean, onResult: (Boolean, String?) -> Unit = { _, _ -> }) {
        val r = room
        if (r == null) {
            onResult(false, "语音未连接")
            return
        }
        val s = scope
        if (s == null) {
            onResult(false, "语音未连接")
            return
        }
        s.launch {
            try {
                r.localParticipant.setMicrophoneEnabled(enabled)
                Log.i(TAG, "microphone enabled=$enabled")
                diagnostics.event("voice", "microphone enabled=$enabled")
                if (scope === s) onResult(true, null)
            } catch (e: CancellationException) {
                // 主动断开时忽略旧连接的取消
            } catch (e: Exception) {
                Log.w(TAG, "microphone toggle failed: ${e.message}")
                diagnostics.event("voice", "microphone toggle failed: ${e.message ?: "unknown"}")
                if (scope === s) onResult(false, e.message ?: "麦克风切换失败")
            }
        }
    }

    fun disconnect() {
        if (room != null || scope != null) diagnostics.event("voice", "disconnect")
        listener = null
        room?.disconnect()
        room = null
        scope?.cancel()
        scope = null
    }
}
