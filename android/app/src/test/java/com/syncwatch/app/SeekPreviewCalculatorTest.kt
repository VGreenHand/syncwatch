package com.syncwatch.app

import org.junit.Assert.assertEquals
import org.junit.Test

class SeekPreviewCalculatorTest {

    @Test
    fun rightHalfSwipeAddsFortyFiveSeconds() {
        val target = SeekPreviewCalculator.targetPositionMs(
            startPositionMs = 60_000L,
            deltaXPx = 500f,
            gestureWidthPx = 1000,
            durationMs = 600_000L
        )
        assertEquals(105_000L, target)
    }

    @Test
    fun leftFullSwipeClampsAtZero() {
        val target = SeekPreviewCalculator.targetPositionMs(
            startPositionMs = 30_000L,
            deltaXPx = -1000f,
            gestureWidthPx = 1000,
            durationMs = 600_000L
        )
        assertEquals(0L, target)
    }

    @Test
    fun rightOverflowClampsAtDuration() {
        val target = SeekPreviewCalculator.targetPositionMs(
            startPositionMs = 590_000L,
            deltaXPx = 300f,
            gestureWidthPx = 1000,
            durationMs = 600_000L
        )
        assertEquals(600_000L, target)
    }

    @Test
    fun noDurationDoesNotSeek() {
        val target = SeekPreviewCalculator.targetPositionMs(
            startPositionMs = 42_000L,
            deltaXPx = 800f,
            gestureWidthPx = 1000,
            durationMs = 0L
        )
        assertEquals(0L, target)
    }
}
