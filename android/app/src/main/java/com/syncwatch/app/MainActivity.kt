package com.syncwatch.app

import android.annotation.SuppressLint
import android.app.Dialog
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.pm.ActivityInfo
import android.content.res.ColorStateList
import android.content.res.Configuration
import android.graphics.Color
import android.graphics.drawable.ColorDrawable
import android.media.AudioManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.provider.OpenableColumns
import android.provider.Settings
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.view.Window
import android.view.WindowManager
import android.view.inputmethod.InputMethodManager
import android.widget.Button
import android.widget.EditText
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.ScrollView
import android.widget.TextView
import androidx.activity.OnBackPressedCallback
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView
import java.util.Locale
import kotlin.math.abs
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt
import kotlin.math.roundToLong

class MainActivity : AppCompatActivity(), SyncEngine.Listener, SyncEngine.PlayerAdapter,
    PlayerControlOverlay.Listener, PlayerGestureController.Listener {

    private lateinit var joinScreen: ScrollView
    private lateinit var playerScreen: View
    private lateinit var homeCard: View
    private lateinit var tabCreate: TextView
    private lateinit var tabJoin: TextView
    private lateinit var etServer: EditText
    private lateinit var etName: EditText
    private lateinit var etRoom: EditText
    private lateinit var etPass: EditText
    private lateinit var roomFields: View
    private lateinit var btnTogglePassword: TextView
    private lateinit var passwordContainer: View
    private lateinit var advancedToggle: View
    private lateinit var advancedFields: View
    private lateinit var advancedChevron: ImageView
    private lateinit var fileCard: View
    private lateinit var fileText: TextView
    private lateinit var fileMeta: TextView
    private lateinit var fileAction: TextView
    private lateinit var formError: TextView
    private lateinit var btnJoin: Button

    private lateinit var videoStage: View
    private lateinit var playerView: PlayerView
    private lateinit var gestureLayer: View
    private lateinit var playerControl: PlayerControlOverlay
    private lateinit var bufferingIndicator: ProgressBar
    private lateinit var playerInfoPanel: ScrollView
    private lateinit var playerTitle: TextView
    private lateinit var statusText: TextView
    private lateinit var statusDot: View
    private lateinit var roomCodeText: TextView
    private lateinit var btnCopyRoom: ImageButton
    private lateinit var membersSummaryRow: View
    private lateinit var membersSummaryText: TextView
    private lateinit var membersChevron: ImageView
    private lateinit var panelMembersList: LinearLayout
    private lateinit var fileWarningText: TextView
    private lateinit var btnPortraitFullscreen: Button
    private lateinit var btnLeaveRoom: TextView
    private lateinit var panelServerText: TextView
    private lateinit var membersDrawer: View
    private lateinit var drawerScrim: View
    private lateinit var drawerPanel: View
    private lateinit var drawerSubtitle: TextView
    private lateinit var drawerMembersList: LinearLayout
    private lateinit var drawerServerText: TextView
    private lateinit var btnCloseDrawer: ImageButton
    private lateinit var connectingOverlay: View
    private lateinit var connectingText: TextView
    private lateinit var lockOverlay: View
    private lateinit var lockHintPanel: View
    private lateinit var btnUnlock: ImageButton
    private lateinit var noticePill: TextView

    private lateinit var player: ExoPlayer
    private var playerInitialized = false
    private var gestureController: PlayerGestureController? = null
    private var engine: SyncEngine? = null

    private var fileUri: Uri? = null
    private var fileName = ""
    private var fileSize = 0L
    private var fileInfoSent = false
    private var lastBuffering = false
    private var currentRoom = ""
    private var currentServer = ""
    private var pendingRequestedRoom = ""
    private var hasJoinedRoom = false
    private var isJoinMode = false
    private var isFullscreen = false
    private var isPlayerVisible = false
    private var isDrawerOpen = false
    private var membersExpanded = false
    private var heartbeatStarted = false
    private var debugAutoplay = false
    private var members: List<MemberInfo> = emptyList()
    private var brightnessBeforePlayer = WindowManager.LayoutParams.BRIGHTNESS_OVERRIDE_NONE
    private var brightnessCaptured = false

    private val mainHandler = Handler(Looper.getMainLooper())
    private val cfg by lazy { getSharedPreferences("cfg", Context.MODE_PRIVATE) }
    private val audioManager by lazy { getSystemService(Context.AUDIO_SERVICE) as AudioManager }

    private val progressTicker = object : Runnable {
        override fun run() {
            if (!playerInitialized || !isPlayerVisible) return
            val position = player.currentPosition.coerceAtLeast(0L)
            val duration = player.duration.coerceAtLeast(0L)
            playerControl.updateProgress(position, duration)
            mainHandler.postDelayed(this, 500L)
        }
    }

    private val noticeHide = Runnable {
        noticePill.animate().cancel()
        noticePill.animate().alpha(0f).setDuration(140L).withEndAction {
            noticePill.visibility = View.GONE
        }.start()
    }

    private val filePicker = registerForActivityResult(
        androidx.activity.result.contract.ActivityResultContracts.OpenDocument()
    ) { uri: Uri? ->
        uri ?: return@registerForActivityResult
        try {
            contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION)
        } catch (_: Exception) {
        }
        fileUri = uri
        val meta = queryFileMeta(uri)
        fileName = meta.first
        fileSize = meta.second
        updateSelectedFileUi()
        clearFormError()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        window.statusBarColor = getColor(R.color.bg_page)
        window.navigationBarColor = getColor(R.color.bg_page)
        window.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_LAYOUT_STABLE

        bindViews()
        setupListeners()

        isJoinMode = cfg.getString("mode", "create") == "join"
        applyModeUi(false)
        etServer.setText(cfg.getString("server", BuildConfig.DEFAULT_SERVER))
        etName.setText(cfg.getString("name", ""))
        etRoom.setText(cfg.getString("room", ""))

        gestureController = PlayerGestureController(this, gestureLayer, this)
        playerControl.listener = this
        playerControl.setSpeed(1f)
        ViewCompat.setOnApplyWindowInsetsListener(videoStage) { _, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars())
            val cutout = insets.getInsets(WindowInsetsCompat.Type.displayCutout())
            val safeLeft = max(bars.left, cutout.left)
            val safeTop = max(bars.top, cutout.top)
            val safeRight = max(bars.right, cutout.right)
            val safeBottom = max(bars.bottom, cutout.bottom)
            if (isFullscreen) {
                playerControl.setSafeInsets(safeLeft, safeTop, safeRight, safeBottom)
            } else {
                playerControl.setSafeInsets(0, 0, 0, 0)
            }
            (lockHintPanel.layoutParams as ViewGroup.MarginLayoutParams).apply {
                marginStart = dp(18) + if (isFullscreen) safeLeft else 0
                lockHintPanel.layoutParams = this
            }
            insets
        }
        bindBackNavigation()
        updateHomeCardWidth()

        handleDebugIntent()
    }

    private fun bindViews() {
        joinScreen = findViewById(R.id.joinScreen)
        playerScreen = findViewById(R.id.playerScreen)
        homeCard = findViewById(R.id.homeCard)
        tabCreate = findViewById(R.id.tabCreate)
        tabJoin = findViewById(R.id.tabJoin)
        etServer = findViewById(R.id.etServer)
        etName = findViewById(R.id.etName)
        etRoom = findViewById(R.id.etRoom)
        etPass = findViewById(R.id.etPass)
        roomFields = findViewById(R.id.roomFields)
        btnTogglePassword = findViewById(R.id.btnTogglePassword)
        passwordContainer = findViewById(R.id.passwordContainer)
        advancedToggle = findViewById(R.id.advancedToggle)
        advancedFields = findViewById(R.id.advancedFields)
        advancedChevron = findViewById(R.id.advancedChevron)
        fileCard = findViewById(R.id.fileCard)
        fileText = findViewById(R.id.fileText)
        fileMeta = findViewById(R.id.fileMeta)
        fileAction = findViewById(R.id.fileAction)
        formError = findViewById(R.id.formError)
        btnJoin = findViewById(R.id.btnJoin)

        videoStage = findViewById(R.id.videoStage)
        playerView = findViewById(R.id.playerView)
        gestureLayer = findViewById(R.id.gestureLayer)
        playerControl = findViewById(R.id.playerControl)
        bufferingIndicator = findViewById(R.id.bufferingIndicator)
        playerInfoPanel = findViewById(R.id.playerInfoPanel)
        playerTitle = findViewById(R.id.playerTitle)
        statusText = findViewById(R.id.statusText)
        statusDot = findViewById(R.id.statusDot)
        roomCodeText = findViewById(R.id.roomCodeText)
        btnCopyRoom = findViewById(R.id.btnCopyRoom)
        membersSummaryRow = findViewById(R.id.membersSummaryRow)
        membersSummaryText = findViewById(R.id.membersSummaryText)
        membersChevron = findViewById(R.id.membersChevron)
        panelMembersList = findViewById(R.id.panelMembersList)
        fileWarningText = findViewById(R.id.fileWarningText)
        btnPortraitFullscreen = findViewById(R.id.btnPortraitFullscreen)
        btnLeaveRoom = findViewById(R.id.btnLeaveRoom)
        panelServerText = findViewById(R.id.panelServerText)
        membersDrawer = findViewById(R.id.membersDrawer)
        drawerScrim = findViewById(R.id.drawerScrim)
        drawerPanel = findViewById(R.id.drawerPanel)
        drawerSubtitle = findViewById(R.id.drawerSubtitle)
        drawerMembersList = findViewById(R.id.drawerMembersList)
        drawerServerText = findViewById(R.id.drawerServerText)
        btnCloseDrawer = findViewById(R.id.btnCloseDrawer)
        connectingOverlay = findViewById(R.id.connectingOverlay)
        connectingText = findViewById(R.id.connectingText)
        lockOverlay = findViewById(R.id.lockOverlay)
        lockHintPanel = findViewById(R.id.lockHintPanel)
        btnUnlock = findViewById(R.id.btnUnlock)
        noticePill = findViewById(R.id.noticePill)
    }

    private fun setupListeners() {
        tabCreate.setOnClickListener { setMode(false) }
        tabJoin.setOnClickListener { setMode(true) }
        fileCard.setOnClickListener { filePicker.launch(arrayOf("video/*")) }
        btnJoin.setOnClickListener { tryJoin() }
        btnTogglePassword.setOnClickListener { togglePasswordField() }
        advancedToggle.setOnClickListener { toggleAdvancedSettings() }
        btnPortraitFullscreen.setOnClickListener { setFullscreen(true) }
        btnLeaveRoom.setOnClickListener { confirmLeaveRoom() }
        btnUnlock.setOnClickListener { setLocked(false) }
        membersSummaryRow.setOnClickListener { toggleMembersExpanded() }
        btnCloseDrawer.setOnClickListener { closeMembersDrawer() }
        drawerScrim.setOnClickListener { closeMembersDrawer() }
        btnCopyRoom.setOnClickListener { copyRoomCode() }
        roomCodeText.setOnClickListener { copyRoomCode() }
    }

    private fun bindBackNavigation() {
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                when {
                    isDrawerOpen -> closeMembersDrawer()
                    !isPlayerVisible -> finish()
                    playerControl.isLocked() -> setLocked(false)
                    isFullscreen -> setFullscreen(false)
                    else -> confirmLeaveRoom()
                }
            }
        })
    }

    // ======================== 首页 ========================

    private fun setMode(join: Boolean) {
        if (isJoinMode == join) return
        isJoinMode = join
        cfg.edit().putString("mode", if (join) "join" else "create").apply()
        applyModeUi(true)
        clearFormError()
    }

    private fun applyModeUi(animate: Boolean) {
        if (animate) {
            homeCard.animate().cancel()
            homeCard.animate().alpha(0.72f).setDuration(80L).withEndAction {
                homeCard.animate().alpha(1f).setDuration(120L).start()
            }.start()
        }
        roomFields.visibility = if (isJoinMode) View.VISIBLE else View.GONE
        btnJoin.text = getString(if (isJoinMode) R.string.action_join else R.string.action_create_start)
        tabCreate.setBackgroundResource(
            if (isJoinMode) R.drawable.bg_segment_unselected else R.drawable.bg_segment_selected
        )
        tabJoin.setBackgroundResource(
            if (isJoinMode) R.drawable.bg_segment_selected else R.drawable.bg_segment_unselected
        )
        tabCreate.setTextColor(getColor(if (isJoinMode) R.color.text_secondary else R.color.text_primary))
        tabJoin.setTextColor(getColor(if (isJoinMode) R.color.text_primary else R.color.text_secondary))
        updatePasswordToggleLabel()
    }

    private fun togglePasswordField() {
        val show = passwordContainer.visibility != View.VISIBLE
        passwordContainer.visibility = if (show) View.VISIBLE else View.GONE
        if (!show) etPass.setText("")
        updatePasswordToggleLabel()
        if (show) etPass.requestFocus()
    }

    private fun updatePasswordToggleLabel() {
        val visible = passwordContainer.visibility == View.VISIBLE
        btnTogglePassword.text = when {
            isJoinMode && visible -> "隐藏密码输入"
            isJoinMode -> "房间有密码？"
            visible -> "取消设置房间密码"
            else -> "设置房间密码（可选）"
        }
    }

    private fun toggleAdvancedSettings() {
        val show = advancedFields.visibility != View.VISIBLE
        advancedFields.visibility = if (show) View.VISIBLE else View.GONE
        advancedChevron.animate().rotation(if (show) 180f else 0f).setDuration(160L).start()
    }

    private fun showFormError(message: String) {
        formError.text = message
        formError.visibility = View.VISIBLE
    }

    private fun clearFormError() {
        if (formError.visibility == View.VISIBLE) formError.visibility = View.GONE
    }

    private fun tryJoin() {
        clearFormError()
        val name = etName.text.toString().trim()
        if (name.isEmpty()) {
            showFormError("请先输入昵称，朋友会在成员列表里看到它")
            etName.requestFocus()
            return
        }
        val uri = fileUri
        if (uri == null) {
            showFormError("请先选择本地视频；房间里每个人都需要使用同一份片源")
            return
        }
        val room = etRoom.text.toString().trim().uppercase()
        if (isJoinMode && room.isEmpty()) {
            showFormError("请输入朋友发来的房间码")
            etRoom.requestFocus()
            return
        }
        val server = normalizeServer(etServer.text.toString())
        if (server.isEmpty()) {
            showFormError("服务器地址不能为空，可在高级设置中填写")
            advancedFields.visibility = View.VISIBLE
            advancedChevron.rotation = 180f
            etServer.requestFocus()
            return
        }

        cfg.edit()
            .putString("server", server)
            .putString("name", name)
            .putString("room", room)
            .apply()

        currentServer = server
        currentRoom = ""
        pendingRequestedRoom = if (isJoinMode) room else ""
        hasJoinedRoom = false
        heartbeatStarted = false
        fileInfoSent = false
        lastBuffering = false

        hideKeyboard()
        setupPlayer(uri)
        updatePlayerChrome()
        isFullscreen = false
        playerControl.setFullscreen(false)
        setConnectionStatus("连接中", R.color.warning)
        updateRoomUi()
        connectingText.text = "正在加入房间…"
        connectingOverlay.visibility = View.VISIBLE
        joinScreen.visibility = View.GONE
        playerScreen.visibility = View.VISIBLE
        isPlayerVisible = true
        applyPlayerLayout()
        startProgressTicker()

        engine = SyncEngine(server, name, this, this).also {
            it.connect(if (isJoinMode) room.ifBlank { null } else null, etPass.text.toString())
        }
    }

    private fun normalizeServer(raw: String): String {
        var value = raw.trim()
            .removePrefix("ws://")
            .removePrefix("wss://")
            .removePrefix("http://")
            .removePrefix("https://")
            .trim()
        if (value.endsWith("/ws")) value = value.dropLast(3)
        return value.trimEnd('/')
    }

    private fun updateSelectedFileUi() {
        if (fileName.isBlank()) {
            fileText.text = getString(R.string.pick_video)
            fileMeta.text = getString(R.string.pick_video_hint)
            fileAction.text = "选择"
            return
        }
        fileText.text = fileName
        fileMeta.text = if (fileSize > 0) "${formatFileSize(fileSize)} · 已选择" else "已选择 · 点击可更换"
        fileAction.text = "更换"
    }

    private fun queryFileMeta(uri: Uri): Pair<String, Long> {
        var name = uri.lastPathSegment ?: ""
        var size = 0L
        try {
            contentResolver.query(uri, null, null, null, null)?.use { cursor ->
                val nameIndex = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                val sizeIndex = cursor.getColumnIndex(OpenableColumns.SIZE)
                if (cursor.moveToFirst()) {
                    if (nameIndex >= 0) name = cursor.getString(nameIndex) ?: name
                    if (sizeIndex >= 0) size = cursor.getLong(sizeIndex)
                }
            }
        } catch (_: Exception) {
        }
        return name to size
    }

    private fun updateHomeCardWidth() {
        val screenWidth = resources.displayMetrics.widthPixels
        val maxWidth = resources.getDimensionPixelSize(R.dimen.home_card_max_width)
        val available = (screenWidth - dp(40)).coerceAtLeast(dp(240))
        val target = min(maxWidth, available)
        val params = homeCard.layoutParams ?: return
        params.width = target
        homeCard.layoutParams = params
    }

    private fun hideKeyboard() {
        val imm = getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager
        imm.hideSoftInputFromWindow(etName.windowToken, 0)
    }

    // ======================== 播放器与连接 ========================

    @SuppressLint("UnsafeOptInUsageError")
    private fun setupPlayer(uri: Uri) {
        releasePlayer()
        if (!brightnessCaptured) {
            brightnessBeforePlayer = window.attributes.screenBrightness
            brightnessCaptured = true
        }
        player = ExoPlayer.Builder(this).build()
        playerInitialized = true
        playerView.player = player
        player.setMediaItem(MediaItem.fromUri(uri))
        player.playWhenReady = false
        player.addListener(playerListener)
        player.prepare()
        playerControl.setSpeed(1f)
        playerControl.resetTransientUi()
        playerControl.setPlaybackState(false, true, 0L)
        bufferingIndicator.visibility = View.VISIBLE
    }

    private fun releasePlayer() {
        if (playerInitialized) {
            runCatching { playerView.player = null }
            runCatching { player.release() }
        }
        playerInitialized = false
        bufferingIndicator.visibility = View.GONE
        lastBuffering = false
    }

    @SuppressLint("UnsafeOptInUsageError")
    private val playerListener = object : Player.Listener {
        override fun onIsPlayingChanged(isPlaying: Boolean) {
            if (!playerInitialized) return
            val e = engine
            if (isPlaying) {
                e?.onLocalPlay()
            } else if (!player.playWhenReady) {
                e?.onLocalPause()
            }
            updatePlaybackStateUi()
        }

        override fun onPlaybackStateChanged(playbackState: Int) {
            if (!playerInitialized) return
            val buffering = playbackState == Player.STATE_BUFFERING
            if (buffering != lastBuffering) {
                lastBuffering = buffering
                bufferingIndicator.visibility = if (buffering && isPlayerVisible) View.VISIBLE else View.GONE
                engine?.onBufferingChanged(buffering)
                if (!buffering) updatePlaybackStateUi()
            }
            if (playbackState == Player.STATE_READY) {
                if (!fileInfoSent && player.duration > 0) {
                    fileInfoSent = true
                    engine?.sendFileInfo(fileName, player.duration / 1000.0)
                }
                updatePlaybackStateUi()
            }
        }

        override fun onPositionDiscontinuity(
            oldPosition: Player.PositionInfo,
            newPosition: Player.PositionInfo,
            reason: Int
        ) {
            if (reason == Player.DISCONTINUITY_REASON_SEEK) {
                engine?.onLocalSeek(newPosition.positionMs / 1000.0)
                if (playerInitialized) {
                    playerControl.updateProgress(
                        newPosition.positionMs,
                        player.duration.coerceAtLeast(0L)
                    )
                }
            }
        }

        override fun onPlaybackParametersChanged(playbackParameters: androidx.media3.common.PlaybackParameters) {
            playerControl.setSpeed(playbackParameters.speed)
        }

        override fun onPlayerError(error: PlaybackException) {
            if (!isPlayerVisible) return
            bufferingIndicator.visibility = View.GONE
            playerControl.setPlaybackState(false, false, player.duration.coerceAtLeast(0L))
            showBlocker(
                "播放失败",
                "无法播放这个文件：${error.errorCodeName}\n\n" +
                    "该格式可能不受支持，建议换用 MP4 / H.264 编码；也请确认文件仍可被本应用读取。"
            )
        }
    }

    private fun updatePlaybackStateUi() {
        if (!playerInitialized) return
        val buffering = player.playbackState == Player.STATE_BUFFERING
        playerControl.setPlaybackState(
            player.isPlaying,
            buffering,
            player.duration.coerceAtLeast(0L)
        )
        playerControl.updateProgress(
            player.currentPosition.coerceAtLeast(0L),
            player.duration.coerceAtLeast(0L)
        )
        bufferingIndicator.visibility = if (buffering && isPlayerVisible) View.VISIBLE else View.GONE
    }

    private fun updatePlayerChrome() {
        val title = fileName.substringBeforeLast('.').ifBlank { "正在播放" }
        playerTitle.text = title
        playerControl.setTitle(title)
        playerControl.setRoomCode(currentRoom)
    }

    private fun updateRoomUi() {
        roomCodeText.text = currentRoom.ifBlank { "——————" }
        playerControl.setRoomCode(currentRoom)
        panelServerText.text = if (currentServer.isBlank()) "" else "服务器 · $currentServer"
        drawerServerText.text = panelServerText.text
    }

    private fun startProgressTicker() {
        mainHandler.removeCallbacks(progressTicker)
        mainHandler.post(progressTicker)
    }

    private fun stopProgressTicker() {
        mainHandler.removeCallbacks(progressTicker)
    }

    private fun setConnectionStatus(text: String, colorRes: Int) {
        statusText.text = text
        statusDot.backgroundTintList = ColorStateList.valueOf(getColor(colorRes))
    }

    override fun onConnected(room: String, created: Boolean) {
        val reconnected = hasJoinedRoom && currentRoom == room
        currentRoom = room
        hasJoinedRoom = true
        connectingOverlay.visibility = View.GONE
        updateRoomUi()
        setConnectionStatus("已连接", R.color.success)
        if (!heartbeatStarted) {
            heartbeatStarted = true
            engine?.startHeartbeat()
        }

        if (created && isJoinMode && pendingRequestedRoom.isNotBlank() && !reconnected) {
            showBlocker(
                "没有加入朋友的房间",
                "服务器上没有房间 ${pendingRequestedRoom}，已临时新建了同码空房 $room。\n\n" +
                    "继续留在这里会和朋友各看各的，请核对房间码后重新加入。"
            ) {
                leaveRoom()
            }
        } else if (reconnected) {
            showNotice("已重新连接房间 $room")
        } else if (created) {
            showNotice("已创建房间 $room，把房间码发给朋友吧")
        } else {
            showNotice("已加入房间 $room")
        }
        pendingRequestedRoom = ""

        if (debugAutoplay) {
            playerControl.postDelayed({
                if (isPlayerVisible && playerInitialized && !player.isPlaying) {
                    player.play()
                }
            }, 2_500L)
        }
    }

    override fun onJoinError(msg: String) {
        leaveRoom()
        showBlocker("没有加入房间", msg)
    }

    override fun onDisconnected() {
        setConnectionStatus("重连中", R.color.warning)
        if (!hasJoinedRoom) connectingText.text = "连接中断，正在重试…"
    }

    override fun onReconnecting(attempt: Int) {
        setConnectionStatus("重连中 $attempt", R.color.warning)
        if (!hasJoinedRoom) connectingText.text = "连接中断，正在进行第 $attempt 次重连…"
    }

    override fun onMembers(members: List<MemberInfo>) {
        this.members = members
        renderMembers()
    }

    override fun onNotice(text: String) {
        showNotice(text.replace("@", " · "))
    }

    override fun onRemoteSeek(position: Double) {
        gestureController?.cancelSeekPreview()
        playerControl.cancelActiveScrub()
    }

    // ======================== PlayerAdapter ========================

    @SuppressLint("UnsafeOptInUsageError")
    override fun play(posSec: Double, rate: Float) {
        if (!playerInitialized) return
        player.playWhenReady = true
        player.playbackParameters = player.playbackParameters.withSpeed(rate)
        updatePlaybackStateUi()
    }

    @SuppressLint("UnsafeOptInUsageError")
    override fun pause(posSec: Double) {
        if (!playerInitialized) return
        player.playWhenReady = false
        updatePlaybackStateUi()
    }

    @SuppressLint("UnsafeOptInUsageError")
    override fun seekTo(posSec: Double) {
        if (!playerInitialized) return
        player.seekTo((posSec * 1000).roundToLong())
        updatePlaybackStateUi()
    }

    override fun currentPos(): Double =
        if (playerInitialized) player.currentPosition / 1000.0 else 0.0

    override fun currentRate(): Float =
        if (playerInitialized) player.playbackParameters.speed else 1f

    override fun isPlaying(): Boolean = playerInitialized && player.isPlaying

    override fun isBuffering(): Boolean =
        playerInitialized && player.playbackState == Player.STATE_BUFFERING

    // ======================== 控制层回调 ========================

    override fun onBack() {
        if (isFullscreen) setFullscreen(false) else confirmLeaveRoom()
    }

    override fun onTogglePlay() {
        if (!playerInitialized) return
        if (player.playWhenReady) {
            player.pause()
        } else {
            if (player.playbackState == Player.STATE_ENDED) player.seekTo(0L)
            player.play()
        }
        updatePlaybackStateUi()
    }

    override fun onSeekRelative(deltaSeconds: Int) {
        if (!playerInitialized) return
        val duration = player.duration
        if (duration <= 0) return
        val target = (player.currentPosition + deltaSeconds * 1000L).coerceIn(0L, duration)
        player.seekTo(target)
        playerControl.updateProgress(target, duration)
        playerControl.revealControls()
    }

    override fun onSeekBarStart() {
        gestureController?.cancelCurrentGesture()
        playerControl.hideSeekPreview()
    }

    override fun onSeekBarPreview(positionMs: Long) {
        // 进度条自身已经展示时间，这里不重复盖中央预览。
    }

    override fun onSeekBarCommit(positionMs: Long) {
        if (!playerInitialized) return
        val duration = player.duration
        if (duration > 0) {
            player.seekTo(positionMs.coerceIn(0L, duration))
            playerControl.updateProgress(positionMs, duration)
        }
    }

    override fun onToggleFullscreen() {
        setFullscreen(!isFullscreen)
    }

    override fun onToggleLock() {
        setLocked(!playerControl.isLocked())
    }

    override fun onOpenMembers() {
        openMembersDrawer()
    }

    override fun onCopyRoom() {
        copyRoomCode()
    }

    override fun onSpeed() {
        showSpeedDialog()
    }

    // ======================== 画面手势回调 ========================

    override fun isFullscreen(): Boolean = isFullscreen

    override fun currentPositionMs(): Long =
        if (playerInitialized) player.currentPosition.coerceAtLeast(0L) else 0L

    override fun durationMs(): Long =
        if (playerInitialized) player.duration.coerceAtLeast(0L) else 0L

    override fun currentBrightness(): Float {
        val windowValue = window.attributes.screenBrightness
        if (windowValue >= 0f) return windowValue.coerceIn(0f, 1f)
        return try {
            Settings.System.getInt(
                contentResolver,
                Settings.System.SCREEN_BRIGHTNESS,
                128
            ).coerceIn(0, 255) / 255f
        } catch (_: Exception) {
            0.5f
        }
    }

    override fun currentVolume(): Float {
        val maxVolume = audioManager.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
        if (maxVolume <= 0) return 0f
        return audioManager.getStreamVolume(AudioManager.STREAM_MUSIC).toFloat() / maxVolume
    }

    override fun onSingleTap() {
        if (!playerControl.isLocked()) playerControl.toggleControls()
    }

    override fun onDoubleTap() {
        if (!playerControl.isLocked()) onTogglePlay()
    }

    override fun onSeekPreview(targetMs: Long, deltaMs: Long) {
        if (!playerInitialized) return
        playerControl.showSeekPreview(
            targetMs,
            player.duration.coerceAtLeast(0L),
            deltaMs
        )
    }

    override fun onSeekCommit(targetMs: Long) {
        if (!playerInitialized) return
        val duration = player.duration
        if (duration <= 0) {
            playerControl.hideSeekPreview()
            return
        }
        player.seekTo(targetMs.coerceIn(0L, duration))
        playerControl.hideSeekPreview()
        playerControl.updateProgress(targetMs.coerceIn(0L, duration), duration)
    }

    override fun onSeekCancel() {
        playerControl.hideSeekPreview()
    }

    override fun onBrightnessChanged(value: Float) {
        val safe = value.coerceIn(0.01f, 1f)
        val params = window.attributes
        params.screenBrightness = safe
        window.attributes = params
        playerControl.showGestureFeedback("亮度", (safe * 100).roundToInt())
    }

    override fun onVolumeChanged(value: Float) {
        val maxVolume = audioManager.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
        if (maxVolume > 0) {
            val target = (value.coerceIn(0f, 1f) * maxVolume).roundToInt()
            audioManager.setStreamVolume(AudioManager.STREAM_MUSIC, target, 0)
            playerControl.showGestureFeedback(
                "音量",
                (target.toFloat() / maxVolume * 100).roundToInt()
            )
        }
    }

    override fun onGestureFinished(wasSeeking: Boolean) {
        if (wasSeeking) {
            playerControl.hideSeekPreview()
        } else {
            mainHandler.postDelayed({ playerControl.hideGestureFeedback() }, 650L)
        }
    }

    // ======================== 全屏、锁定、面板 ========================

    private fun setFullscreen(fullscreen: Boolean) {
        if (!isPlayerVisible) return
        isFullscreen = fullscreen
        membersDrawer.visibility = View.GONE
        isDrawerOpen = false
        playerControl.setPanelOpen(false)
        playerControl.setFullscreen(fullscreen)
        requestedOrientation = if (fullscreen) {
            ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
        } else {
            ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
        }
        applySystemBars(fullscreen)
        if (!fullscreen) playerControl.setSafeInsets(0, 0, 0, 0)
        ViewCompat.requestApplyInsets(videoStage)
        playerInfoPanel.visibility = if (fullscreen) View.GONE else View.VISIBLE
        playerControl.revealControls()
        playerScreen.post { applyPlayerLayout() }
    }

    private fun applySystemBars(fullscreen: Boolean) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            val attrs = window.attributes
            attrs.layoutInDisplayCutoutMode = if (fullscreen) {
                WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES
            } else {
                WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_DEFAULT
            }
            window.attributes = attrs
        }
        val controller = WindowCompat.getInsetsController(window, window.decorView)
        if (fullscreen) {
            WindowCompat.setDecorFitsSystemWindows(window, false)
            controller.systemBarsBehavior =
                WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            controller.hide(WindowInsetsCompat.Type.systemBars())
        } else {
            controller.show(WindowInsetsCompat.Type.systemBars())
            WindowCompat.setDecorFitsSystemWindows(window, true)
            window.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_LAYOUT_STABLE
        }
        window.statusBarColor = getColor(R.color.bg_page)
        window.navigationBarColor = if (fullscreen) getColor(R.color.black) else getColor(R.color.bg_page)
        ViewCompat.requestApplyInsets(window.decorView)
    }

    private fun applyPlayerLayout() {
        if (!isPlayerVisible) return
        val width = playerScreen.width
        val height = playerScreen.height
        if (width <= 0 || height <= 0) {
            playerScreen.post { applyPlayerLayout() }
            return
        }

        val stageParams = videoStage.layoutParams as android.widget.FrameLayout.LayoutParams
        val infoParams = playerInfoPanel.layoutParams as android.widget.FrameLayout.LayoutParams

        if (isFullscreen) {
            stageParams.width = android.widget.FrameLayout.LayoutParams.MATCH_PARENT
            stageParams.height = android.widget.FrameLayout.LayoutParams.MATCH_PARENT
            stageParams.leftMargin = 0
            stageParams.topMargin = 0
            playerInfoPanel.visibility = View.GONE
        } else {
            playerInfoPanel.visibility = View.VISIBLE
            val isTablet = resources.configuration.screenWidthDp >= 600
            if (isTablet) {
                val side = min(dp(360), max(dp(300), (width * 0.36f).roundToInt()))
                val stageWidth = (width - side).coerceAtLeast(dp(320))
                stageParams.width = stageWidth
                stageParams.height = android.widget.FrameLayout.LayoutParams.MATCH_PARENT
                stageParams.leftMargin = 0
                stageParams.topMargin = 0
                infoParams.width = width - stageWidth
                infoParams.height = android.widget.FrameLayout.LayoutParams.MATCH_PARENT
                infoParams.leftMargin = stageWidth
                infoParams.topMargin = 0
            } else {
                val videoHeight = min((width * 9f / 16f).roundToInt(), (height * 0.58f).roundToInt())
                stageParams.width = android.widget.FrameLayout.LayoutParams.MATCH_PARENT
                stageParams.height = videoHeight
                stageParams.leftMargin = 0
                stageParams.topMargin = 0
                infoParams.width = android.widget.FrameLayout.LayoutParams.MATCH_PARENT
                infoParams.height = android.widget.FrameLayout.LayoutParams.MATCH_PARENT
                infoParams.leftMargin = 0
                infoParams.topMargin = videoHeight
            }
        }
        videoStage.layoutParams = stageParams
        playerInfoPanel.layoutParams = infoParams
        updateDrawerWidth(width)
    }

    private fun updateDrawerWidth(screenWidth: Int) {
        val params = drawerPanel.layoutParams
        params.width = min(dp(360), (screenWidth * 0.88f).roundToInt())
        drawerPanel.layoutParams = params
    }

    private fun setLocked(locked: Boolean) {
        playerControl.setLocked(locked)
        lockOverlay.visibility = if (locked) View.VISIBLE else View.GONE
        gestureController?.cancelCurrentGesture()
        closeMembersDrawer()
        if (!locked) {
            playerControl.revealControls()
        }
    }

    private fun openMembersDrawer() {
        if (!isPlayerVisible || isDrawerOpen) return
        isDrawerOpen = true
        playerControl.setPanelOpen(true)
        updateDrawerWidth(playerScreen.width.coerceAtLeast(dp(320)))
        renderMembers()
        membersDrawer.visibility = View.VISIBLE
        membersDrawer.alpha = 0f
        drawerPanel.translationX = dp(36).toFloat()
        membersDrawer.animate().alpha(1f).setDuration(140L).start()
        drawerPanel.animate().translationX(0f).setDuration(190L).start()
    }

    private fun closeMembersDrawer() {
        if (!isDrawerOpen) {
            membersDrawer.visibility = View.GONE
            return
        }
        isDrawerOpen = false
        playerControl.setPanelOpen(false)
        drawerPanel.animate().translationX(dp(36).toFloat()).setDuration(140L).start()
        membersDrawer.animate().alpha(0f).setDuration(140L).withEndAction {
            if (!isDrawerOpen) membersDrawer.visibility = View.GONE
        }.start()
    }

    private fun toggleMembersExpanded() {
        membersExpanded = !membersExpanded
        panelMembersList.visibility = if (membersExpanded) View.VISIBLE else View.GONE
        membersChevron.animate()
            .rotation(if (membersExpanded) 180f else 0f)
            .setDuration(150L)
            .start()
    }

    // ======================== 成员与房间信息 ========================

    private fun renderMembers() {
        val online = members.size
        val buffering = members.count { it.state == "buffering" }
        val summary = when {
            online == 0 -> "在线成员"
            online == 1 -> "1 人在线 · 等待朋友加入"
            buffering > 0 -> "$online 人在线 · $buffering 人缓冲中"
            else -> "$online 人在线 · 全员就绪"
        }
        membersSummaryText.text = summary
        drawerSubtitle.text = summary

        panelMembersList.removeAllViews()
        drawerMembersList.removeAllViews()
        members.forEach { member ->
            panelMembersList.addView(buildMemberRow(member))
            drawerMembersList.addView(buildMemberRow(member))
        }

        val durations = members.map { it.duration }.filter { it > 0.0 }
        if (durations.size >= 2 && (durations.max() - durations.min()) > 2.0) {
            val detail = members
                .filter { it.duration > 0.0 }
                .joinToString(" · ") { "${it.name} ${formatDurationSeconds(it.duration)}" }
            fileWarningText.text = "片源时长不一致，进度会越看越偏：\n$detail"
            fileWarningText.visibility = View.VISIBLE
        } else {
            fileWarningText.visibility = View.GONE
        }
    }

    private fun buildMemberRow(member: MemberInfo): View {
        val row = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = android.view.Gravity.CENTER_VERTICAL
            setPadding(dp(12), dp(10), dp(12), dp(10))
            background = ContextCompat.getDrawable(this@MainActivity, R.drawable.bg_input)
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
            ).apply { bottomMargin = dp(6) }
        }
        val dot = View(this).apply {
            layoutParams = LinearLayout.LayoutParams(dp(7), dp(7))
            background = ContextCompat.getDrawable(this@MainActivity, R.drawable.dot_connected)
            backgroundTintList = ColorStateList.valueOf(getColor(stateColor(member.state)))
        }
        row.addView(dot)

        val name = TextView(this).apply {
            text = member.name + if (member.name == etName.text.toString().trim()) "（你）" else ""
            setTextColor(getColor(R.color.text_primary))
            textSize = 15f
            maxLines = 1
            ellipsize = android.text.TextUtils.TruncateAt.END
            layoutParams = LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f)
                .apply { marginStart = dp(10) }
        }
        row.addView(name)

        if (member.duration > 0.0) {
            val duration = TextView(this).apply {
                text = formatDurationSeconds(member.duration)
                setTextColor(getColor(R.color.text_hint))
                textSize = 11f
                layoutParams = LinearLayout.LayoutParams(
                    LinearLayout.LayoutParams.WRAP_CONTENT,
                    LinearLayout.LayoutParams.WRAP_CONTENT
                ).apply { marginEnd = dp(10) }
            }
            row.addView(duration)
        }

        val state = TextView(this).apply {
            text = stateCn(member.state)
            setTextColor(getColor(stateColor(member.state)))
            textSize = 12f
        }
        row.addView(state)
        return row
    }

    private fun stateColor(state: String): Int = when (state) {
        "playing" -> R.color.success
        "buffering" -> R.color.warning
        else -> R.color.text_secondary
    }

    private fun stateCn(state: String): String = when (state) {
        "playing" -> "播放中"
        "buffering" -> "缓冲中"
        else -> "暂停"
    }

    private fun copyRoomCode() {
        if (currentRoom.isBlank()) return
        val clipboard = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        clipboard.setPrimaryClip(ClipData.newPlainText("room", currentRoom))
        showNotice("房间码 $currentRoom 已复制")
    }

    private fun formatDurationSeconds(seconds: Double): String =
        formatDuration((seconds * 1000).roundToLong())

    private fun formatDuration(ms: Long): String {
        val totalSeconds = (ms / 1000.0).roundToInt().coerceAtLeast(0)
        val sec = totalSeconds % 60
        val totalMinutes = totalSeconds / 60
        val minute = totalMinutes % 60
        val hour = totalMinutes / 60
        return if (hour > 0) {
            String.format(Locale.CHINA, "%d:%02d:%02d", hour, minute, sec)
        } else {
            String.format(Locale.CHINA, "%02d:%02d", minute, sec)
        }
    }

    // ======================== 速度、离开与对话框 ========================

    private fun showSpeedDialog() {
        if (!playerInitialized) return
        val labels = arrayOf("0.5×", "0.75×", "1.0×", "1.25×", "1.5×", "2.0×")
        val rates = floatArrayOf(0.5f, 0.75f, 1f, 1.25f, 1.5f, 2f)
        val current = player.playbackParameters.speed
        val checked = rates.indexOfFirst { abs(it - current) < 0.01f }.coerceAtLeast(2)

        val dialog = Dialog(this)
        val container = createDialogContainer()
        container.addView(createDialogTitle("播放倍速"))

        val options = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
            ).apply { topMargin = dp(12) }
        }
        labels.forEachIndexed { index, label ->
            val selected = index == checked
            val row = TextView(this).apply {
                text = if (selected) "✓   $label" else "      $label"
                setTextColor(getColor(if (selected) R.color.brand_light else R.color.text_primary))
                textSize = 15f
                gravity = Gravity.CENTER_VERTICAL
                setPadding(dp(12), 0, dp(12), 0)
                background = ContextCompat.getDrawable(this@MainActivity, R.drawable.bg_dialog_action)
                isClickable = true
                isFocusable = true
                layoutParams = LinearLayout.LayoutParams(
                    LinearLayout.LayoutParams.MATCH_PARENT,
                    dp(48)
                )
                setOnClickListener {
                    dialog.dismiss()
                    applyPlaybackSpeed(rates[index])
                }
            }
            options.addView(row)
        }
        container.addView(options)

        val actions = createDialogActionsRow()
        actions.addView(createDialogAction("取消", R.color.text_secondary) { dialog.dismiss() })
        container.addView(actions)

        dialog.setContentView(container)
        configureDialogWindow(dialog, cancelable = true)
        dialog.show()
    }

    private fun dialogRoot(): LinearLayout = LinearLayout(this).apply {
        orientation = LinearLayout.VERTICAL
        background = ContextCompat.getDrawable(this@MainActivity, R.drawable.bg_dialog)
        setPadding(dp(22), dp(22), dp(22), dp(16))
    }

    private fun createDialogContainer(): LinearLayout = dialogRoot()

    private fun createDialogTitle(title: String): TextView = TextView(this).apply {
        text = title
        setTextColor(getColor(R.color.text_primary))
        textSize = 20f
        setTypeface(typeface, android.graphics.Typeface.BOLD)
    }

    private fun createDialogMessage(message: String): TextView = TextView(this).apply {
        text = message
        setTextColor(getColor(R.color.text_secondary))
        textSize = 14f
        setLineSpacing(dp(3).toFloat(), 1f)
        layoutParams = LinearLayout.LayoutParams(
            LinearLayout.LayoutParams.MATCH_PARENT,
            LinearLayout.LayoutParams.WRAP_CONTENT
        ).apply { topMargin = dp(10) }
    }

    private fun createDialogActionsRow(): LinearLayout = LinearLayout(this).apply {
        orientation = LinearLayout.HORIZONTAL
        gravity = Gravity.END
        layoutParams = LinearLayout.LayoutParams(
            LinearLayout.LayoutParams.MATCH_PARENT,
            LinearLayout.LayoutParams.WRAP_CONTENT
        ).apply { topMargin = dp(20) }
    }

    private fun createDialogAction(
        text: String,
        colorRes: Int,
        onClick: () -> Unit
    ): TextView = TextView(this).apply {
        this.text = text
        setTextColor(getColor(colorRes))
        textSize = 14f
        setTypeface(typeface, android.graphics.Typeface.BOLD)
        gravity = Gravity.CENTER
        setPadding(dp(16), 0, dp(16), 0)
        minHeight = dp(44)
        minWidth = dp(56)
        background = ContextCompat.getDrawable(this@MainActivity, R.drawable.bg_dialog_action)
        isClickable = true
        isFocusable = true
        setOnClickListener { onClick() }
    }

    private fun configureDialogWindow(dialog: Dialog, cancelable: Boolean) {
        dialog.setCancelable(cancelable)
        dialog.setCanceledOnTouchOutside(cancelable)
        dialog.window?.apply {
            setBackgroundDrawable(ColorDrawable(Color.TRANSPARENT))
            addFlags(WindowManager.LayoutParams.FLAG_DIM_BEHIND)
            setDimAmount(0.72f)
        }
        dialog.setOnShowListener {
            val width = min(dp(430), resources.displayMetrics.widthPixels - dp(48))
            dialog.window?.setLayout(width, ViewGroup.LayoutParams.WRAP_CONTENT)
        }
    }

    @SuppressLint("UnsafeOptInUsageError")
    private fun applyPlaybackSpeed(rate: Float) {
        if (!playerInitialized) return
        player.playbackParameters = player.playbackParameters.withSpeed(rate)
        playerControl.setSpeed(rate)
        if (player.isPlaying) {
            engine?.onLocalRateChanged(rate)
        } else {
            showNotice("已设置 ${String.format(Locale.CHINA, "%.1f", rate)}×，继续播放后同步")
        }
    }

    private fun confirmLeaveRoom() {
        if (!isPlayerVisible) return
        showSyncDialog(
            title = "离开房间？",
            message = "离开后将断开与当前房间的同步。你的本地视频不会被上传或删除。",
            positiveText = "离开房间",
            negativeText = "继续观看",
            positiveColorRes = R.color.error,
            onPositive = { leaveRoom() }
        )
    }

    private fun leaveRoom(restoreOrientation: Boolean = true) {
        if (!isPlayerVisible) return
        stopProgressTicker()
        closeMembersDrawer()
        gestureController?.cancelCurrentGesture()
        playerControl.setPanelOpen(false)
        playerControl.setLocked(false)
        playerControl.resetTransientUi()
        lockOverlay.visibility = View.GONE
        membersDrawer.visibility = View.GONE
        isDrawerOpen = false

        engine?.close()
        engine = null
        releasePlayer()
        currentRoom = ""
        pendingRequestedRoom = ""
        hasJoinedRoom = false
        heartbeatStarted = false
        fileInfoSent = false
        isPlayerVisible = false
        isFullscreen = false
        playerControl.setFullscreen(false)
        playerScreen.visibility = View.GONE
        joinScreen.visibility = View.VISIBLE
        connectingOverlay.visibility = View.GONE
        mainHandler.removeCallbacks(noticeHide)
        noticePill.visibility = View.GONE

        if (restoreOrientation) {
            requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
        }
        applySystemBars(false)
        if (brightnessCaptured) {
            val params = window.attributes
            params.screenBrightness = brightnessBeforePlayer
            window.attributes = params
            brightnessCaptured = false
        }
        updateRoomUi()
        updateHomeCardWidth()
    }

    private fun showBlocker(title: String, message: String, onDismiss: (() -> Unit)? = null) {
        showSyncDialog(
            title = title,
            message = message,
            positiveText = "知道了",
            cancelable = false,
            onPositive = onDismiss
        )
    }

    private fun showSyncDialog(
        title: String,
        message: String? = null,
        positiveText: String = "知道了",
        negativeText: String? = null,
        positiveColorRes: Int = R.color.brand_light,
        cancelable: Boolean = true,
        onNegative: (() -> Unit)? = null,
        onPositive: (() -> Unit)? = null
    ) {
        val dialog = Dialog(this)
        val container = createDialogContainer()
        container.addView(createDialogTitle(title))
        if (!message.isNullOrBlank()) container.addView(createDialogMessage(message))

        val actions = createDialogActionsRow()
        if (negativeText != null) {
            actions.addView(createDialogAction(negativeText, R.color.text_secondary) {
                dialog.dismiss()
                onNegative?.invoke()
            })
        }
        actions.addView(createDialogAction(positiveText, positiveColorRes) {
            dialog.dismiss()
            onPositive?.invoke()
        })
        container.addView(actions)

        dialog.setContentView(container)
        configureDialogWindow(dialog, cancelable)
        dialog.show()
    }

    private fun showNotice(text: String) {
        if (text.isBlank()) return
        mainHandler.removeCallbacks(noticeHide)
        noticePill.animate().cancel()
        noticePill.text = text
        noticePill.setTextColor(getColor(R.color.text_primary))
        noticePill.visibility = View.VISIBLE
        noticePill.alpha = 0f
        noticePill.animate().alpha(1f).setDuration(120L).start()
        mainHandler.postDelayed(noticeHide, 2_200L)
    }

    // ======================== 生命周期与调试入口 ========================

    private fun handleDebugIntent() {
        if (!BuildConfig.DEBUG) return
        val video = intent?.getStringExtra("video") ?: return
        val f = if (video.startsWith("/")) java.io.File(video) else java.io.File(filesDir, video)
        if (!f.exists()) {
            showFormError("调试视频不存在：${f.absolutePath}")
            return
        }
        intent?.getStringExtra("server")?.takeIf { it.isNotBlank() }?.let { etServer.setText(it) }
        intent?.getStringExtra("name")?.takeIf { it.isNotBlank() }?.let { etName.setText(it) }
        intent?.getStringExtra("room")?.takeIf { it.isNotBlank() }?.let {
            etRoom.setText(it)
            if (!isJoinMode) {
                isJoinMode = true
                applyModeUi(false)
            }
        }
        fileUri = Uri.fromFile(f)
        fileName = f.name
        fileSize = f.length()
        updateSelectedFileUi()
        debugAutoplay = intent?.getBooleanExtra("autoplay", false) == true
        if (intent?.getBooleanExtra("autojoin", false) == true) {
            fileCard.post { tryJoin() }
        }
    }

    override fun onNewIntent(intent: Intent?) {
        super.onNewIntent(intent)
        setIntent(intent)
        if (isPlayerVisible) leaveRoom()
        handleDebugIntent()
    }

    override fun onConfigurationChanged(newConfig: Configuration) {
        super.onConfigurationChanged(newConfig)
        updateHomeCardWidth()
        if (isPlayerVisible) {
            playerScreen.post { applyPlayerLayout() }
            applySystemBars(isFullscreen)
        }
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (hasFocus && isPlayerVisible && isFullscreen) {
            applySystemBars(true)
        }
    }

    override fun onStop() {
        super.onStop()
        if (playerInitialized && isPlayerVisible && player.playWhenReady) {
            player.pause()
        }
    }

    override fun onDestroy() {
        mainHandler.removeCallbacksAndMessages(null)
        engine?.close()
        engine = null
        releasePlayer()
        super.onDestroy()
    }

    private fun dp(value: Int): Int = (value * resources.displayMetrics.density).roundToInt()

    private fun formatFileSize(bytes: Long): String = when {
        bytes >= 1024L * 1024L * 1024L ->
            String.format(Locale.CHINA, "%.1f GB", bytes / 1024.0 / 1024.0 / 1024.0)
        bytes >= 1024L * 1024L ->
            String.format(Locale.CHINA, "%.1f MB", bytes / 1024.0 / 1024.0)
        bytes >= 1024L -> String.format(Locale.CHINA, "%.0f KB", bytes / 1024.0)
        else -> "$bytes B"
    }
}
