package com.syncwatch.app

import android.annotation.SuppressLint
import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.provider.OpenableColumns
import android.view.View
import android.view.WindowManager
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView

class MainActivity : AppCompatActivity(), SyncEngine.Listener, SyncEngine.PlayerAdapter {

    private lateinit var player: ExoPlayer
    private lateinit var playerView: PlayerView
    private lateinit var joinLayout: LinearLayout
    private lateinit var playerLayout: LinearLayout
    private lateinit var membersText: TextView
    private lateinit var roomText: TextView
    private lateinit var etServer: EditText
    private lateinit var etName: EditText
    private lateinit var etRoom: EditText
    private lateinit var etPass: EditText
    private lateinit var fileText: TextView

    private var engine: SyncEngine? = null
    private var fileUri: Uri? = null
    private var fileName = ""
    private var fileInfoSent = false
    private var lastBuffering = false
    private var currentRoom = ""

    private val filePicker = registerForActivityResult(
        androidx.activity.result.contract.ActivityResultContracts.OpenDocument()
    ) { uri: Uri? ->
        uri ?: return@registerForActivityResult
        try {
            contentResolver.takePersistableUriPermission(
                uri, Intent.FLAG_GRANT_READ_URI_PERMISSION
            )
        } catch (_: Exception) {}
        fileUri = uri
        fileName = queryName(uri)
        fileText.text = fileName.ifBlank { "已选择视频文件" }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)

        joinLayout = findViewById(R.id.joinLayout)
        playerLayout = findViewById(R.id.playerLayout)
        etServer = findViewById(R.id.etServer)
        etName = findViewById(R.id.etName)
        etRoom = findViewById(R.id.etRoom)
        etPass = findViewById(R.id.etPass)
        fileText = findViewById(R.id.fileText)
        membersText = findViewById(R.id.membersText)
        roomText = findViewById(R.id.roomText)
        roomText.setOnClickListener { copyRoomCode() }
        playerView = findViewById(R.id.playerView)

        etServer.setText(getSharedPreferences("cfg", MODE_PRIVATE).getString("server", BuildConfig.DEFAULT_SERVER))
        etName.setText(getSharedPreferences("cfg", MODE_PRIVATE).getString("name", ""))
        etRoom.setText(getSharedPreferences("cfg", MODE_PRIVATE).getString("room", ""))

        findViewById<View>(R.id.btnPick).setOnClickListener {
            filePicker.launch(arrayOf("video/*"))
        }
        findViewById<View>(R.id.btnJoin).setOnClickListener { tryJoin() }
    }

    private fun tryJoin() {
        val name = etName.text.toString().trim()
        if (name.isEmpty()) { toast("请输入昵称"); return }
        if (fileUri == null) { toast("请先选择视频文件"); return }

        getSharedPreferences("cfg", MODE_PRIVATE).edit().apply {
            putString("server", etServer.text.toString().trim())
            putString("name", name)
            putString("room", etRoom.text.toString().trim())
            apply()
        }

        initPlayer()
        engine = SyncEngine(
            etServer.text.toString().trim(), name, this, this
        ).also {
            it.connect(etRoom.text.toString().ifBlank { null }, etPass.text.toString())
        }

        joinLayout.visibility = View.GONE
        playerLayout.visibility = View.VISIBLE
    }

    @SuppressLint("UnsafeOptInUsageError")
    private fun initPlayer() {
        player = ExoPlayer.Builder(this).build().apply {
            setMediaItem(MediaItem.fromUri(fileUri!!))
            playWhenReady = false
            addListener(playerListener)
            prepare()
        }
        playerView.player = player
    }

    @SuppressLint("UnsafeOptInUsageError")
    private val playerListener = object : Player.Listener {
        override fun onIsPlayingChanged(isPlaying: Boolean) {
            engine?.let { if (isPlaying) it.onLocalPlay() else it.onLocalPause() }
        }

        override fun onPlaybackStateChanged(playbackState: Int) {
            if (playbackState == Player.STATE_BUFFERING) {
                if (!lastBuffering) { lastBuffering = true; engine?.onBufferingChanged(true) }
            } else if (playbackState == Player.STATE_READY) {
                if (lastBuffering) { lastBuffering = false; engine?.onBufferingChanged(false) }
                if (!fileInfoSent && player.duration > 0) {
                    fileInfoSent = true
                    engine?.sendFileInfo(fileName, player.duration / 1000.0)
                }
            }
        }

        override fun onPositionDiscontinuity(
            oldPosition: Player.PositionInfo,
            newPosition: Player.PositionInfo,
            reason: Int
        ) {
            if (reason == Player.DISCONTINUITY_REASON_SEEK) {
                engine?.onLocalSeek(newPosition.positionMs / 1000.0)
            }
        }

        override fun onPlayerError(error: PlaybackException) {
            toast("播放出错: ${error.errorCodeName}（该格式可能不受支持，请换 MP4/H.264 编码的文件）")
        }
    }

    // ---------- PlayerAdapter ----------

    @SuppressLint("UnsafeOptInUsageError")
    override fun play(posSec: Double, rate: Float) {
        player.playWhenReady = true
        player.playbackParameters = player.playbackParameters.withSpeed(rate)
    }

    @SuppressLint("UnsafeOptInUsageError")
    override fun pause(posSec: Double) {
        player.playWhenReady = false
    }

    @SuppressLint("UnsafeOptInUsageError")
    override fun seekTo(posSec: Double) {
        player.seekTo((posSec * 1000).toLong())
    }

    override fun currentPos(): Double = player.currentPosition / 1000.0
    override fun currentRate(): Float = player.playbackParameters.speed
    override fun isPlaying(): Boolean = player.isPlaying
    @SuppressLint("UnsafeOptInUsageError")
    override fun isBuffering(): Boolean = player.playbackState == Player.STATE_BUFFERING

    // ---------- SyncEngine.Listener ----------

    override fun onConnected(room: String, created: Boolean) {
        currentRoom = room
        roomText.text = "房间码 $room · 点击复制"
        if (created) {
            val requested = etRoom.text.toString().trim()
            if (requested.isNotEmpty() && !requested.equals(room, ignoreCase = true)) {
                // 填了码却新建了房间 = 那个房间不存在，必须点破，否则两人各看各的
                Toast.makeText(this, "房间码 $requested 不存在，已新建房间 $room，请核对朋友发来的码", Toast.LENGTH_LONG).show()
            } else {
                toast("已创建房间 $room（把房间码发给朋友）")
            }
        } else {
            toast("已加入房间 $room")
        }
        engine?.startHeartbeat()
    }

    /** 点顶部房间码即可复制，不用手抄那 6 位 */
    private fun copyRoomCode() {
        if (currentRoom.isEmpty()) return
        val cm = getSystemService(CLIPBOARD_SERVICE) as android.content.ClipboardManager
        cm.setPrimaryClip(android.content.ClipData.newPlainText("room", currentRoom))
        toast("房间码 $currentRoom 已复制")
    }

    override fun onJoinError(msg: String) {
        toast("加入失败: $msg")
        finishPlayer()
    }

    override fun onMembers(members: List<MemberInfo>) {
        membersText.text = if (members.isEmpty()) {
            "暂无成员"
        } else {
            "在线 ${members.size} 人 · " + members.joinToString("  ") { "${it.name}(${stateCn(it.state)})" }
        }
    }

    override fun onNotice(text: String) {
        Toast.makeText(this, text, Toast.LENGTH_SHORT).show()
    }

    override fun onDisconnected() {}

    // ---------- 工具 ----------

    private fun stateCn(s: String) = when (s) {
        "playing" -> "播放中"; "buffering" -> "缓冲中"; else -> "暂停"
    }

    private fun queryName(uri: Uri): String = try {
        contentResolver.query(uri, null, null, null, null)?.use { c ->
            val idx = c.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            if (idx >= 0 && c.moveToFirst()) c.getString(idx) else uri.lastPathSegment ?: ""
        } ?: ""
    } catch (_: Exception) { "" }

    private fun toast(s: String) = Toast.makeText(this, s, Toast.LENGTH_SHORT).show()

    private fun finishPlayer() {
        engine?.close()
        engine = null
        if (::player.isInitialized) player.release()
        currentRoom = ""
        roomText.text = "房间码 —"
        joinLayout.visibility = View.VISIBLE
        playerLayout.visibility = View.GONE
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        if (playerLayout.visibility == View.VISIBLE) {
            finishPlayer()
        } else {
            super.onBackPressed()
        }
    }

    override fun onStop() {
        super.onStop()
        // 观影场景屏幕常亮，进入后台即暂停并同步状态
        if (::player.isInitialized && playerLayout.visibility == View.VISIBLE) {
            player.playWhenReady = false
        }
    }

    override fun onDestroy() {
        engine?.close()
        if (::player.isInitialized) player.release()
        super.onDestroy()
    }
}
