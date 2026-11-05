package com.syncwatch.app

import org.json.JSONArray
import org.json.JSONObject

// 与服务端一致的协议消息
data class MemberInfo(
    val name: String,
    val state: String, // playing/paused/buffering
    val file: String = "",
    val duration: Double = 0.0
)

object Protocol {
    fun join(room: String?, name: String, password: String): String =
        JSONObject().apply {
            put("type", "join")
            if (!room.isNullOrBlank()) put("room", room.trim().uppercase())
            put("name", name.trim())
            put("password", password)
        }.toString()

    fun play(pos: Double, rate: Float) = JSONObject().apply {
        put("type", "play"); put("position", pos); put("rate", rate.toDouble())
    }.toString()

    fun pause(pos: Double) = JSONObject().apply {
        put("type", "pause"); put("position", pos)
    }.toString()

    fun seek(pos: Double) = JSONObject().apply {
        put("type", "seek"); put("position", pos)
    }.toString()

    fun heartbeat(pos: Double, rate: Float) = JSONObject().apply {
        put("type", "heartbeat"); put("position", pos); put("rate", rate.toDouble())
    }.toString()

    fun memberState(state: String) = JSONObject().apply {
        put("type", "memberState"); put("state", state)
    }.toString()

    fun fileInfo(file: String, duration: Double) = JSONObject().apply {
        put("type", "fileInfo"); put("file", file); put("duration", duration)
    }.toString()

    fun leave() = JSONObject().apply { put("type", "leave") }.toString()

    fun parseMembers(arr: JSONArray?): List<MemberInfo> {
        val list = mutableListOf<MemberInfo>()
        arr ?: return list
        for (i in 0 until arr.length()) {
            val o = arr.getJSONObject(i)
            list.add(MemberInfo(
                o.optString("name"),
                o.optString("state", "paused"),
                o.optString("file", ""),
                o.optDouble("duration", 0.0)
            ))
        }
        return list
    }
}
