package com.syncwatch.app

import kotlin.math.roundToLong

/**
 * 横滑进度的纯计算逻辑，和触摸、播放器完全解耦，便于单元测试。
 */
object SeekPreviewCalculator {
    const val FULL_WIDTH_SEEK_MS = 90_000L

    fun targetPositionMs(
        startPositionMs: Long,
        deltaXPx: Float,
        gestureWidthPx: Int,
        durationMs: Long,
        fullWidthSeekMs: Long = FULL_WIDTH_SEEK_MS
    ): Long {
        if (gestureWidthPx <= 0 || durationMs <= 0L) {
            return startPositionMs.coerceIn(0L, durationMs.coerceAtLeast(0L))
        }
        val deltaMs = (deltaXPx.toDouble() / gestureWidthPx.toDouble() * fullWidthSeekMs)
            .roundToLong()
        return (startPositionMs + deltaMs).coerceIn(0L, durationMs)
    }

    fun deltaMs(targetMs: Long, startMs: Long): Long = targetMs - startMs
}
