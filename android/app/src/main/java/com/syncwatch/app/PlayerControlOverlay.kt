package com.syncwatch.app

import android.content.Context
import android.content.res.ColorStateList
import android.os.Handler
import android.os.Looper
import android.util.AttributeSet
import android.view.LayoutInflater
import android.view.View
import android.widget.FrameLayout
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.SeekBar
import android.widget.TextView
import java.util.Locale
import kotlin.math.roundToInt

/**
 * 播放页的自定义控制层。
 *
 * 只负责展示与用户意图，不直接操作 ExoPlayer 或 SyncEngine；
 * 所有实际动作通过 [Listener] 交回 MainActivity，避免 UI 和同步链路耦合。
 */
class PlayerControlOverlay @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
    defStyleAttr: Int = 0
) : FrameLayout(context, attrs, defStyleAttr) {

    interface Listener {
        fun onBack()
        fun onTogglePlay()
        fun onSeekRelative(deltaSeconds: Int)
        fun onSeekBarStart()
        fun onSeekBarPreview(positionMs: Long)
        fun onSeekBarCommit(positionMs: Long)
        fun onToggleFullscreen()
        fun onToggleLock()
        fun onOpenMembers()
        fun onCopyRoom()
        fun onSpeed()
        fun onToggleVoiceMic()
    }

    var listener: Listener? = null

    private val topBar: View
    private val bottomBar: View
    private val centerControls: View
    private val titleView: TextView
    private val roomBadge: TextView
    private val btnBack: ImageButton
    private val btnPlayPause: ImageButton
    private val btnReplay10: View
    private val btnForward10: View
    private val btnLock: ImageButton
    private val btnMembers: ImageButton
    private val btnFullscreen: ImageButton
    private val btnVoice: ImageButton
    private val btnSpeed: TextView
    private val seekBar: SeekBar
    private val currentTime: TextView
    private val totalTime: TextView
    private val seekPreviewCard: View
    private val previewTargetTime: TextView
    private val previewDelta: TextView
    private val gestureFeedback: View
    private val feedbackValue: TextView
    private val feedbackLabel: TextView

    private val hideHandler = Handler(Looper.getMainLooper())
    private val hideControlsRunnable = Runnable { hideControls() }

    private var durationMs = 0L
    private var isPlaying = false
    private var isBuffering = false
    private var isScrubbing = false
    private var cancelScrubOnRelease = false
    private var isFullscreen = false
    private var isLocked = false
    private var isPanelOpen = false
    private var speed = 1.0f

    init {
        LayoutInflater.from(context).inflate(R.layout.view_player_controller, this, true)

        topBar = findViewById(R.id.controllerTopBar)
        bottomBar = findViewById(R.id.controllerBottomBar)
        centerControls = findViewById(R.id.centerControls)
        titleView = findViewById(R.id.controllerTitle)
        roomBadge = findViewById(R.id.controllerRoomBadge)
        btnBack = findViewById(R.id.btnControllerBack)
        btnPlayPause = findViewById(R.id.btnPlayPause)
        btnReplay10 = findViewById(R.id.btnReplay10)
        btnForward10 = findViewById(R.id.btnForward10)
        btnLock = findViewById(R.id.btnControllerLock)
        btnMembers = findViewById(R.id.btnControllerMembers)
        btnFullscreen = findViewById(R.id.btnFullscreen)
        btnVoice = findViewById(R.id.btnVoice)
        btnSpeed = findViewById(R.id.btnSpeed)
        seekBar = findViewById(R.id.playerSeekBar)
        currentTime = findViewById(R.id.currentTime)
        totalTime = findViewById(R.id.totalTime)
        seekPreviewCard = findViewById(R.id.seekPreviewCard)
        previewTargetTime = findViewById(R.id.previewTargetTime)
        previewDelta = findViewById(R.id.previewDelta)
        gestureFeedback = findViewById(R.id.gestureFeedback)
        feedbackValue = findViewById(R.id.feedbackValue)
        feedbackLabel = findViewById(R.id.feedbackLabel)

        btnBack.setOnClickListener {
            revealControls()
            listener?.onBack()
        }
        btnPlayPause.setOnClickListener {
            revealControls()
            listener?.onTogglePlay()
        }
        btnReplay10.setOnClickListener {
            revealControls()
            listener?.onSeekRelative(-10)
        }
        btnForward10.setOnClickListener {
            revealControls()
            listener?.onSeekRelative(10)
        }
        btnLock.setOnClickListener {
            listener?.onToggleLock()
        }
        btnMembers.setOnClickListener {
            revealControls()
            listener?.onOpenMembers()
        }
        btnFullscreen.setOnClickListener {
            revealControls()
            listener?.onToggleFullscreen()
        }
        btnSpeed.setOnClickListener {
            revealControls()
            listener?.onSpeed()
        }
        btnVoice.setOnClickListener {
            revealControls()
            listener?.onToggleVoiceMic()
        }
        roomBadge.setOnClickListener {
            listener?.onCopyRoom()
        }

        seekBar.setOnSeekBarChangeListener(object : SeekBar.OnSeekBarChangeListener {
            override fun onProgressChanged(bar: SeekBar?, progress: Int, fromUser: Boolean) {
                if (!fromUser) return
                val target = progressToPosition(progress)
                currentTime.text = formatTime(target)
                listener?.onSeekBarPreview(target)
            }

            override fun onStartTrackingTouch(bar: SeekBar?) {
                isScrubbing = true
                cancelScrubOnRelease = false
                cancelAutoHide()
                listener?.onSeekBarStart()
            }

            override fun onStopTrackingTouch(bar: SeekBar?) {
                isScrubbing = false
                if (!cancelScrubOnRelease) {
                    val target = progressToPosition(bar?.progress ?: 0)
                    listener?.onSeekBarCommit(target)
                }
                cancelScrubOnRelease = false
                scheduleAutoHide()
            }
        })

        setFullscreen(false)
        updatePlayIcon()
    }

    fun setTitle(title: String) {
        titleView.text = title.ifBlank { "正在播放" }
    }

    fun setRoomCode(code: String) {
        roomBadge.text = if (code.isBlank()) "房间码 —" else "房间码 $code"
    }

    fun setVoiceVisible(visible: Boolean) {
        btnVoice.visibility = if (visible) View.VISIBLE else View.GONE
    }

    fun setMicOn(on: Boolean) {
        btnVoice.setImageResource(if (on) R.drawable.ic_mic else R.drawable.ic_mic_off)
        btnVoice.contentDescription = if (on) "关闭麦克风" else "打开麦克风"
        btnVoice.imageTintList = ColorStateList.valueOf(
            context.getColor(if (on) R.color.brand_light else R.color.text_secondary)
        )
    }

    fun setSpeed(rate: Float) {
        speed = rate
        btnSpeed.text = when {
            rate == rate.toInt().toFloat() -> String.format(Locale.CHINA, "%.1f×", rate)
            else -> String.format(Locale.CHINA, "%s×", rate.toString().trimEnd('0').trimEnd('.'))
        }
    }

    fun setPlaybackState(playing: Boolean, buffering: Boolean, duration: Long = durationMs) {
        isPlaying = playing
        isBuffering = buffering
        durationMs = duration.coerceAtLeast(0L)
        updatePlayIcon()
        if (playing && !buffering && !isScrubbing && !isPanelOpen) {
            scheduleAutoHide()
        } else {
            cancelAutoHide()
            if (!isLocked && !isPanelOpen) revealControls()
        }
    }

    fun updateProgress(positionMs: Long, duration: Long) {
        durationMs = duration.coerceAtLeast(0L)
        if (durationMs > 0) {
            totalTime.text = formatTime(durationMs)
        } else {
            totalTime.text = "00:00"
        }
        if (!isScrubbing) {
            currentTime.text = formatTime(positionMs.coerceAtLeast(0L))
            seekBar.isEnabled = durationMs > 0
            if (durationMs > 0) {
                seekBar.progress = ((positionMs.toDouble() / durationMs) * 1000)
                    .roundToInt()
                    .coerceIn(0, 1000)
            } else {
                seekBar.progress = 0
            }
        }
    }

    fun setFullscreen(fullscreen: Boolean) {
        isFullscreen = fullscreen
        roomBadge.visibility = if (fullscreen) VISIBLE else GONE
        btnLock.visibility = if (fullscreen) VISIBLE else GONE
        btnMembers.visibility = if (fullscreen) VISIBLE else GONE
        btnFullscreen.setImageResource(
            if (fullscreen) R.drawable.ic_fullscreen_exit else R.drawable.ic_fullscreen
        )
        btnFullscreen.contentDescription = if (fullscreen) "退出全屏" else "进入全屏"
    }

    fun isFullscreen(): Boolean = isFullscreen

    fun setLocked(locked: Boolean) {
        isLocked = locked
        if (locked) {
            cancelAutoHide()
            hideControlsImmediately()
        } else {
            setVisibility(VISIBLE)
            alpha = 1f
            revealControls()
        }
    }

    fun isLocked(): Boolean = isLocked

    fun setPanelOpen(open: Boolean) {
        isPanelOpen = open
        if (open) {
            cancelAutoHide()
        } else if (isPlaying && !isBuffering && !isScrubbing) {
            scheduleAutoHide()
        }
    }

    /**
     * 全屏 edge-to-edge 后把安全区还给顶部与底部控制，避免刘海/系统栏压住按钮。
     * 非全屏时传 0，由正常窗口 inset 负责。
     */
    fun setSafeInsets(left: Int, top: Int, right: Int, bottom: Int) {
        topBar.setPadding(dp(6) + left, top, dp(6) + right, 0)
        val topParams = topBar.layoutParams
        topParams.height = dp(56) + top
        topBar.layoutParams = topParams
        bottomBar.setPadding(dp(12) + left, dp(4), dp(12) + right, dp(6) + bottom)
    }

    fun controlsVisible(): Boolean = visibility == VISIBLE && alpha > 0.05f

    fun revealControls() {
        if (isLocked) return
        hideHandler.removeCallbacks(hideControlsRunnable)
        visibility = VISIBLE
        animate().cancel()
        animate().alpha(1f).setDuration(140L).start()
        scheduleAutoHide()
    }

    fun toggleControls() {
        if (isLocked) return
        if (controlsVisible()) {
            hideControls()
        } else {
            revealControls()
        }
    }

    fun hideControls() {
        if (isLocked || isScrubbing || isPanelOpen) return
        if (!isPlaying || isBuffering) return
        if (!controlsVisible()) return
        hideHandler.removeCallbacks(hideControlsRunnable)
        animate().cancel()
        animate().alpha(0f).setDuration(150L).withEndAction {
            if (!isLocked) visibility = INVISIBLE
        }.start()
    }

    private fun hideControlsImmediately() {
        animate().cancel()
        alpha = 0f
        visibility = INVISIBLE
    }

    private fun scheduleAutoHide() {
        hideHandler.removeCallbacks(hideControlsRunnable)
        if (isPlaying && !isBuffering && !isScrubbing && !isPanelOpen && !isLocked) {
            hideHandler.postDelayed(hideControlsRunnable, AUTO_HIDE_MS)
        }
    }

    private fun cancelAutoHide() {
        hideHandler.removeCallbacks(hideControlsRunnable)
    }

    fun showSeekPreview(targetMs: Long, duration: Long, deltaMs: Long) {
        cancelAutoHide()
        durationMs = duration.coerceAtLeast(0L)
        val safeDelta = kotlin.math.abs(deltaMs)
        previewTargetTime.text = "${formatTime(targetMs)} / ${formatTime(durationMs)}"
        previewDelta.text = if (deltaMs >= 0) {
            "快进 +${formatClock(safeDelta)}"
        } else {
            "快退 −${formatClock(safeDelta)}"
        }
        centerControls.visibility = INVISIBLE
        seekPreviewCard.visibility = VISIBLE
        gestureFeedback.visibility = GONE
        if (!controlsVisible()) {
            visibility = VISIBLE
            animate().cancel()
            alpha = 1f
        }
    }

    fun hideSeekPreview() {
        seekPreviewCard.visibility = GONE
        centerControls.visibility = VISIBLE
        scheduleAutoHide()
    }

    fun showGestureFeedback(label: String, value: Int) {
        cancelAutoHide()
        feedbackLabel.text = label
        feedbackValue.text = "$value%"
        centerControls.visibility = INVISIBLE
        gestureFeedback.visibility = VISIBLE
        seekPreviewCard.visibility = GONE
        if (!controlsVisible()) {
            visibility = VISIBLE
            animate().cancel()
            alpha = 1f
        }
    }

    fun hideGestureFeedback() {
        gestureFeedback.visibility = GONE
        centerControls.visibility = VISIBLE
        scheduleAutoHide()
    }

    fun resetTransientUi() {
        seekPreviewCard.visibility = GONE
        gestureFeedback.visibility = GONE
        centerControls.visibility = VISIBLE
        centerControls.alpha = 1f
    }

    fun setResponseWaiting(waiting: Boolean) {
        centerControls.alpha = if (waiting) 0.45f else 1f
        btnReplay10.isEnabled = !waiting
        btnForward10.isEnabled = !waiting
        btnPlayPause.isEnabled = !waiting
    }

    /**
     * 进度条拖动期间如果收到远端 seek，远端位置优先。
     * 松手时不再把拖动开始时的旧目标写回房间。
     */
    fun cancelActiveScrub() {
        if (isScrubbing) cancelScrubOnRelease = true
    }

    fun currentRateLabel(): String = btnSpeed.text.toString()

    private fun updatePlayIcon() {
        btnPlayPause.setImageResource(if (isPlaying) R.drawable.ic_pause else R.drawable.ic_play)
        btnPlayPause.contentDescription = if (isPlaying) "暂停" else "播放"
    }

    private fun progressToPosition(progress: Int): Long {
        if (durationMs <= 0) return 0L
        return (durationMs * (progress.coerceIn(0, 1000) / 1000.0)).toLong()
    }

    private fun formatTime(ms: Long): String = formatClock(ms.coerceAtLeast(0L))

    private fun formatClock(rawMs: Long): String {
        val totalSeconds = (rawMs / 1000.0).roundToInt().coerceAtLeast(0)
        val seconds = totalSeconds % 60
        val minutesTotal = totalSeconds / 60
        val minutes = minutesTotal % 60
        val hours = minutesTotal / 60
        return if (hours > 0) {
            String.format(Locale.CHINA, "%d:%02d:%02d", hours, minutes, seconds)
        } else {
            String.format(Locale.CHINA, "%02d:%02d", minutes, seconds)
        }
    }

    override fun onDetachedFromWindow() {
        hideHandler.removeCallbacks(hideControlsRunnable)
        animate().cancel()
        super.onDetachedFromWindow()
    }

    private fun dp(value: Int): Int = (value * resources.displayMetrics.density).roundToInt()

    companion object {
        private const val AUTO_HIDE_MS = 3_000L
    }
}
