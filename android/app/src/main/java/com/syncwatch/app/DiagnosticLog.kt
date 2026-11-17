package com.syncwatch.app

import android.content.Context
import android.os.Build
import android.util.Log
import java.io.File
import java.io.FileOutputStream
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * 本机诊断日志：事件按天写入应用私有目录，只在用户主动导出时交付。
 * 不记录密码、token 正文、密钥或设备唯一标识。
 */
class DiagnosticLog(context: Context) {
    companion object {
        private const val TAG = "DiagnosticLog"
        private const val DIR_NAME = "diagnostics"
        private const val EXPORT_DIR_NAME = "diagnostics-export"
        private const val RETENTION_MS = 7L * 24 * 60 * 60 * 1000
        private const val MAX_TOTAL_BYTES = 2L * 1024 * 1024
        private const val MAX_FILE_BYTES = 512L * 1024
    }

    private val appContext = context.applicationContext
    private val logDir = File(appContext.filesDir, DIR_NAME)
    private val lock = Any()
    private val lineTimeFormat = SimpleDateFormat("yyyy-MM-dd HH:mm:ss.SSS", Locale.US)
    private val dayFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)
    private val exportStampFormat = SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US)

    @Volatile private var lastRoom = ""
    @Volatile private var lastNickname = ""
    @Volatile private var lastServer = ""

    init {
        synchronized(lock) {
            try {
                ensureDir()
                cleanupLocked(System.currentTimeMillis())
            } catch (e: Exception) {
                Log.w(TAG, "init failed: ${e.message}")
            }
        }
    }

    fun setContext(room: String? = null, nickname: String? = null, server: String? = null) {
        var changed = false
        var snapshot = ""
        synchronized(lock) {
            if (room != null && room != lastRoom) {
                lastRoom = room
                changed = true
            }
            if (nickname != null && nickname != lastNickname) {
                lastNickname = nickname
                changed = true
            }
            if (server != null && server != lastServer) {
                lastServer = server
                changed = true
            }
            snapshot = "room=${lastRoom.ifBlank { "-" }} " +
                "nickname=${lastNickname.ifBlank { "-" }} " +
                "server=${lastServer.ifBlank { "-" }}"
        }
        if (changed) {
            event("session", snapshot)
        }
    }

    fun event(category: String, message: String) {
        val now = System.currentTimeMillis()
        val safeMessage = message
            .replace('\r', ' ')
            .replace('\n', ' ')
            .take(2_000)

        synchronized(lock) {
            try {
                val line = "${lineTimeFormat.format(Date(now))} " +
                    "[${category.take(18)}] $safeMessage\n"
                val bytes = line.toByteArray(Charsets.UTF_8)
                ensureDir()
                cleanupLocked(now)
                FileOutputStream(appendTargetLocked(now, bytes.size), true).use {
                    it.write(bytes)
                }
                trimLocked()
            } catch (e: Exception) {
                Log.w(TAG, "write failed: ${e.message}")
            }
        }
    }

    data class ExportMeta(
        val room: String,
        val nickname: String,
        val server: String
    )

    fun export(meta: ExportMeta): Result<File> {
        synchronized(lock) {
            try {
                ensureDir()
                cleanupLocked(System.currentTimeMillis())
                val files = logDir.listFiles()
                    ?.filter { it.isFile && it.name.startsWith("diag-") && it.name.endsWith(".log") }
                    ?.sortedBy { it.lastModified() }
                    .orEmpty()
                if (files.isEmpty()) {
                    return Result.failure(IllegalStateException("本机还没有诊断日志"))
                }

                val exportDir = File(appContext.cacheDir, EXPORT_DIR_NAME).apply { mkdirs() }
                exportDir.listFiles()?.forEach { it.delete() }
                val out = File(
                    exportDir,
                    "syncwatch-diagnostics-${exportStampFormat.format(Date())}.txt"
                )

                out.bufferedWriter(Charsets.UTF_8).use { writer ->
                    writer.write(buildHeader(meta))
                    files.forEach { file ->
                        writer.write("\n--- ${file.name} ---\n")
                        file.bufferedReader(Charsets.UTF_8).useLines { lines ->
                            lines.forEach {
                                writer.write(it)
                                writer.write("\n")
                            }
                        }
                    }
                }
                return Result.success(out)
            } catch (e: Exception) {
                return Result.failure(e)
            }
        }
    }

    private fun ensureDir() {
        if (!logDir.exists() && !logDir.mkdirs()) {
            throw IllegalStateException("无法创建日志目录")
        }
    }

    private fun appendTargetLocked(now: Long, nextBytes: Int): File {
        val day = dayFormat.format(Date(now))
        val base = File(logDir, "diag-$day.log")
        if (!base.exists() || base.length() == 0L) return base
        if (base.length() + nextBytes <= MAX_FILE_BYTES) return base

        var index = 1
        while (true) {
            val rotated = File(logDir, "diag-$day-$index.log")
            if (!rotated.exists() || rotated.length() == 0L) return rotated
            if (rotated.length() + nextBytes <= MAX_FILE_BYTES) return rotated
            index++
        }
    }

    private fun cleanupLocked(now: Long) {
        val cutoff = now - RETENTION_MS
        logDir.listFiles()?.forEach { file ->
            if (file.isFile && file.lastModified() < cutoff) file.delete()
        }
        trimLocked()
    }

    private fun trimLocked() {
        val files = logDir.listFiles()?.filter { it.isFile }.orEmpty()
        var total = files.sumOf { it.length() }
        if (total <= MAX_TOTAL_BYTES) return

        files.sortedWith(compareBy<File> { it.lastModified() }.thenBy { it.name })
            .forEach { file ->
                if (total <= MAX_TOTAL_BYTES) return
                total -= file.length()
                file.delete()
            }
    }

    private fun buildHeader(meta: ExportMeta): String {
        val now = lineTimeFormat.format(Date())
        val apk = "${BuildConfig.VERSION_NAME} (${BuildConfig.VERSION_CODE}) " +
            if (BuildConfig.DEBUG) "debug" else "release"
        val device = "${Build.MANUFACTURER} ${Build.MODEL}".trim()
        val os = "Android ${Build.VERSION.RELEASE} (API ${Build.VERSION.SDK_INT})"
        return buildString {
            appendLine("SyncWatch 诊断日志")
            appendLine("导出时间: $now")
            appendLine("APK: $apk")
            appendLine("机型: $device")
            appendLine("系统: $os")
            appendLine("昵称: ${meta.nickname.ifBlank { lastNickname }.ifBlank { "-" }}")
            appendLine("当前/最近房间: ${meta.room.ifBlank { lastRoom }.ifBlank { "-" }}")
            appendLine("服务器: ${meta.server.ifBlank { lastServer }.ifBlank { "-" }}")
            appendLine("说明: 日志仅在本机生成；含服务器地址，请只发送给开发端，不要公开传播。")
            appendLine("=== 事件日志 ===")
        }
    }
}
