package com.syncwatch.app

import android.content.Context
import android.view.GestureDetector
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import kotlin.math.abs
import kotlin.math.max

/**
 * 播放画面手势状态机。
 *
 * 关键规则：
 * - 滑动时只预览，不逐帧 seek；
 * - 松手后只提交一次目标位置；
 * - 收到远端 seek 时 [cancelSeekPreview] 会让本次手势的后续 ACTION_UP 失效；
 * - 亮度/音量只在全屏启用，且一次触摸只进入一种手势。
 */
class PlayerGestureController(
    context: Context,
    private val touchView: View,
    private val listener: Listener
) {

    interface Listener {
        fun isFullscreen(): Boolean
        fun currentPositionMs(): Long
        fun durationMs(): Long
        fun currentBrightness(): Float
        fun currentVolume(): Float

        fun onSingleTap()
        fun onDoubleTap()
        fun onSeekPreview(targetMs: Long, deltaMs: Long)
        fun onSeekCommit(targetMs: Long)
        fun onSeekCancel()
        fun onBrightnessChanged(value: Float)
        fun onVolumeChanged(value: Float)
        fun onGestureFinished(wasSeeking: Boolean)
    }

    private enum class Mode { NONE, SEEK, BRIGHTNESS, VOLUME, IGNORE }

    private val touchSlop = ViewConfiguration.get(context).scaledTouchSlop.toFloat()
    private var mode = Mode.NONE
    private var downX = 0f
    private var downY = 0f
    private var startPositionMs = 0L
    private var targetPositionMs = 0L
    private var lastTargetPositionMs = 0L
    private var startBrightness = 0f
    private var startVolume = 0f
    private var currentBrightness = 0f
    private var currentVolume = 0f
    private var suppressUntilUp = false
    private var gestureActive = false

    private val gestureDetector = GestureDetector(context, object : GestureDetector.SimpleOnGestureListener() {
        override fun onDown(e: MotionEvent): Boolean = true

        override fun onSingleTapConfirmed(e: MotionEvent): Boolean {
            listener.onSingleTap()
            touchView.performClick()
            return true
        }

        override fun onDoubleTap(e: MotionEvent): Boolean {
            listener.onDoubleTap()
            return true
        }
    })

    init {
        touchView.isClickable = true
        touchView.setOnTouchListener { _, event -> handleTouch(event) }
    }

    private fun handleTouch(event: MotionEvent): Boolean {
        gestureDetector.onTouchEvent(event)

        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                mode = Mode.NONE
                suppressUntilUp = false
                gestureActive = true
                downX = event.x
                downY = event.y
                startPositionMs = listener.currentPositionMs().coerceAtLeast(0L)
                targetPositionMs = startPositionMs
                lastTargetPositionMs = startPositionMs
                startBrightness = listener.currentBrightness().coerceIn(0f, 1f)
                startVolume = listener.currentVolume().coerceIn(0f, 1f)
                currentBrightness = startBrightness
                currentVolume = startVolume
            }

            MotionEvent.ACTION_MOVE -> {
                if (!gestureActive || suppressUntilUp) return true
                val dx = event.x - downX
                val dy = event.y - downY

                if (mode == Mode.NONE) {
                    if (max(abs(dx), abs(dy)) <= touchSlop) return true
                    mode = when {
                        abs(dx) > abs(dy) -> Mode.SEEK
                        listener.isFullscreen() && abs(dy) > abs(dx) -> {
                            if (downX < touchView.width / 2f) Mode.BRIGHTNESS else Mode.VOLUME
                        }
                        else -> Mode.IGNORE
                    }
                }

                when (mode) {
                    Mode.SEEK -> updateSeek(dx)
                    Mode.BRIGHTNESS -> updateBrightness(dy)
                    Mode.VOLUME -> updateVolume(dy)
                    else -> Unit
                }
            }

            MotionEvent.ACTION_UP -> {
                if (gestureActive) {
                    if (mode == Mode.SEEK && !suppressUntilUp) {
                        listener.onSeekCommit(targetPositionMs)
                    }
                    listener.onGestureFinished(mode == Mode.SEEK)
                }
                resetGesture()
                return true
            }

            MotionEvent.ACTION_CANCEL -> {
                if (mode == Mode.SEEK && !suppressUntilUp) {
                    listener.onSeekCancel()
                }
                if (gestureActive) listener.onGestureFinished(mode == Mode.SEEK)
                resetGesture()
                return true
            }
        }
        return true
    }

    private fun updateSeek(dx: Float) {
        targetPositionMs = SeekPreviewCalculator.targetPositionMs(
            startPositionMs = startPositionMs,
            deltaXPx = dx,
            gestureWidthPx = touchView.width,
            durationMs = listener.durationMs()
        )
        if (targetPositionMs == lastTargetPositionMs) return
        lastTargetPositionMs = targetPositionMs
        listener.onSeekPreview(
            targetPositionMs,
            SeekPreviewCalculator.deltaMs(targetPositionMs, startPositionMs)
        )
    }

    private fun updateBrightness(dy: Float) {
        currentBrightness = adjustedVerticalValue(startBrightness, dy)
        listener.onBrightnessChanged(currentBrightness)
    }

    private fun updateVolume(dy: Float) {
        currentVolume = adjustedVerticalValue(startVolume, dy)
        listener.onVolumeChanged(currentVolume)
    }

    private fun adjustedVerticalValue(start: Float, dy: Float): Float {
        val basis = touchView.height.coerceAtLeast(1) * 0.65f
        return (start - dy / basis).coerceIn(0f, 1f)
    }

    /**
     * 远端 seek 优先。取消后，本次触摸余下的事件不再生效，防止松手时把旧目标写回去。
     */
    fun cancelSeekPreview() {
        if (mode == Mode.SEEK) {
            suppressUntilUp = true
            listener.onSeekCancel()
        }
    }

    fun cancelCurrentGesture() {
        if (mode == Mode.SEEK) listener.onSeekCancel()
        suppressUntilUp = true
        mode = Mode.NONE
        gestureActive = false
    }

    private fun resetGesture() {
        mode = Mode.NONE
        suppressUntilUp = false
        gestureActive = false
    }
}
