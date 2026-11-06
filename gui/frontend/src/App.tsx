import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import './App.css'
import {
  ChooseVideo,
  ClearLogs,
  CopyText,
  CreateRoom,
  GetConfig,
  GetLogs,
  DebugIntent,
  GetState,
  JoinRoom,
  LeaveRoom,
  LogMsg,
  LoadVideo,
  PingServer,
  SaveConfig,
  Seek,
  SetRate,
  SetVideoRect,
  SetVolume,
  ShowVideoHost,
  ToggleFullscreen,
  TogglePlay,
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import {
  IconAlert, IconArrowRight, IconBack, IconCheck, IconClose, IconCopy,
  IconExitFullscreen, IconFilm, IconFullscreen, IconLink, IconLog,
  IconMute, IconPause, IconPlay, IconPlus, IconSettings, IconUsers, IconVolume,
} from './Icons'

type Page = 'home' | 'create' | 'join' | 'player'
type Panel = null | 'members' | 'settings' | 'logs'

type Member = { name: string; state: string; file?: string; duration?: number; online: boolean }
type Video = { path: string; name: string; size: number }
type Notice = { id: number; text: string; kind: 'info' | 'warn' | 'error' }

const fmtTime = (s: number) => {
  if (!isFinite(s) || s < 0) s = 0
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = Math.floor(s % 60)
  const mm = String(m).padStart(2, '0')
  const ss = String(sec).padStart(2, '0')
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`
}

const fmtSize = (n: number) => {
  if (!n) return ''
  if (n > 1024 ** 3) return (n / 1024 ** 3).toFixed(1) + ' GB'
  if (n > 1024 ** 2) return (n / 1024 ** 2).toFixed(0) + ' MB'
  return (n / 1024).toFixed(0) + ' KB'
}

const stateCn = (s: string) => {
  switch (s) {
    case 'playing': return '正在播放'
    case 'paused': return '已暂停'
    case 'buffering': return '正在缓冲'
    default: return '在线'
  }
}

const stateDot = (s: string) => {
  switch (s) {
    case 'playing': return 'dot-play'
    case 'paused': return 'dot-pause'
    case 'buffering': return 'dot-buffer'
    default: return 'dot-idle'
  }
}

// 回环地址只有本机能连——朋友永远接不上。
// 两个人各自连着自己的服务器时，界面都显示"已连接"，但互相看不到对方，
// 这正是 v0.5 之后最容易被误判成"同步坏了"的情况。
const isLoopback = (s?: string) => {
  const h = (s || '').trim().toLowerCase()
  return h.startsWith('127.') || h.startsWith('localhost') || h.startsWith('0.0.0.0') || h.startsWith('[::1]') || h.startsWith('::1')
}

export default function App() {
  const [page, setPage] = useState<Page>('home')
  const [panel, setPanel] = useState<Panel>(null)

  const [server, setServer] = useState('')
  const [name, setName] = useState('')
  const [video, setVideo] = useState<Video | null>(null)
  const [roomInput, setRoomInput] = useState('')
  const [password, setPassword] = useState('')

  const [room, setRoom] = useState('')
  const [conn, setConn] = useState('idle')
  const [members, setMembers] = useState<Member[]>([])
  const [pos, setPos] = useState(0)
  const [dur, setDur] = useState(0)
  const [paused, setPaused] = useState(true)
  const [rate, setRate] = useState(1)
  const [volume, setVolume] = useState(100)
  const [full, setFull] = useState(false)
  const [scrubbing, setScrubbing] = useState(false)
  const [ctrlVisible, setCtrlVisible] = useState(true)
  const [osd, setOsd] = useState('')
  // 加入房间时如果服务器告诉我们"这房间本来不存在"，说明进错了地方，
  // 必须阻断式提示，不能只用 Toast（设计稿 4.3）。
  const [joinWarn, setJoinWarn] = useState<{ room: string; server: string } | null>(null)

  const [notices, setNotices] = useState<Notice[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [logs, setLogs] = useState('')
  const [serverCheck, setServerCheck] = useState('')
  const [confirmLeave, setConfirmLeave] = useState(false)
  const [version, setVersion] = useState('')

  const videoRef = useRef<HTMLDivElement>(null)
  const hideTimer = useRef<number>(0)
  const noticeId = useRef(0)
  const osdTimer = useRef<number>(0)
  const clickTimer = useRef<number>(0)
  // 方向键连按时，用它累积目标进度：mpv 跳转有延迟，
  // 不能每次都拿"当前播放位置"当基准，否则按住右键会原地打转。
  const seekPending = useRef<number | null>(null)
  const seekTimer = useRef<number>(0)
  // 本次点的是"加入"还是"创建"，用来判断 hello.created 是否意味着进错房间
  const joinAttempt = useRef('')

  const toast = useCallback((text: string, kind: Notice['kind'] = 'info') => {
    const id = ++noticeId.current
    setNotices((prev) => [...prev.slice(-3), { id, text, kind }])
    window.setTimeout(() => setNotices((prev) => prev.filter((n) => n.id !== id)), 4000)
  }, [])

  // 轻量 OSD：调音量 / 快进快退时在画面下方闪一下，不堆通知
  const showOsd = useCallback((text: string) => {
    setOsd(text)
    window.clearTimeout(osdTimer.current)
    osdTimer.current = window.setTimeout(() => setOsd(''), 1200)
  }, [])

  // 把视频区矩形同步给 Go，Go 据此摆放 mpv 宿主子窗口
  const syncRect = useCallback(() => {
    const el = videoRef.current
    if (!el || page !== 'player') return
    const r = el.getBoundingClientRect()
    SetVideoRect(r.left, r.top, r.width, r.height, window.innerWidth, window.innerHeight)
  }, [page])

  // 进入播放页才让 mpv 画面出现，回到首页则隐藏
  useEffect(() => {
    if (page === 'player') ShowVideoHost(true)
    else ShowVideoHost(false)
  }, [page])

  useEffect(() => {
    syncRect()
    const el = videoRef.current
    const ro = new ResizeObserver(() => syncRect())
    if (el) ro.observe(el)
    window.addEventListener('resize', syncRect)
    const t = window.setInterval(syncRect, 700)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', syncRect)
      window.clearInterval(t)
    }
  }, [syncRect])

  // 自动化联调入口：带环境变量启动时自动载片并进房，正常使用不会走到
  useEffect(() => {
    LogMsg('界面已挂载')
    DebugIntent().then(async (d: any) => {
      LogMsg('调试意图: ' + JSON.stringify(d))
      if (!d?.video) return
      const nm = d.name || '测试端'
      const srv = d.server || '127.0.0.1:18998'
      setName(nm)
      setServer(srv)
      try {
        const v: any = await LoadVideo(d.video)
        if (v) setVideo({ path: v.path, name: v.name, size: v.size })
        // 调试入口只在内存里改服务器地址，绝不写 config.json：
        // 以前这里会 SaveConfig，把 127.0.0.1 永久写进用户配置，
        // 于是分发包自带的公网地址被顶掉，两个人各自连到不同服务器。
        await SaveConfig({ name: nm, lastVideo: d.video } as any)
        if (d.room === 'new') {
          joinAttempt.current = ''
          await CreateRoom(nm, '')
        } else if (d.room) {
          joinAttempt.current = d.room
          setRoomInput(d.room)
          await JoinRoom(d.room, nm, '')
        }
        // 联调用的额外动作：seek=秒数，toggle=1 表示播放/暂停一次
        if (d.seek) {
          window.setTimeout(() => { Seek(Number(d.seek)).catch(() => { }) }, 1500)
        }
        if (d.toggle === '1') {
          window.setTimeout(() => { TogglePlay().catch(() => { }) }, 1800)
        }
      } catch (e: any) {
        setError(String(e))
      }
    }).catch((e: any) => LogMsg('调试入口失败: ' + String(e)))
  }, [])

  useEffect(() => {
    GetConfig().then((c: any) => {
      if (c?.server) setServer(c.server)
      if (c?.name) setName(c.name)
    }).catch(() => { })
    GetState().then((s: any) => { if (s?.version) setVersion(String(s.version)) }).catch(() => { })
    EventsOn('host:error', (m: string) => setError(String(m)))
  }, [])

  // 房间与播放器事件
  useEffect(() => {
    const offs = [
      EventsOn('room:joined', (d: any) => {
        const code = d?.room || ''
        setRoom(code)
        setConn('connected')
        setPage('player')
        setBusy(false)
        if (Array.isArray(d?.members)) setMembers(d.members)

        if (d?.again) {
          // 断线重连回来的 hello：不要再说"已加入房间"
          toast(`已重新连上房间 ${code}`)
          joinAttempt.current = ''
          return
        }
        const asked = joinAttempt.current
        joinAttempt.current = ''
        if (asked && (d?.created || String(asked).toUpperCase() !== code)) {
          // 填了房间码，服务器却新建了房间 —— 说明这个码在**这台服务器**上不存在。
          // 房间码打错、房间已过期、或者两个人连的根本不是同一台服务器，都会走到这里。
          // 这种时候界面必须挡住用户，否则两边都显示"已连接"，却谁也看不到谁。
          setJoinWarn({ room: code, server })
        } else if (d?.created) {
          toast(`已创建房间 ${code}，把房间码发给朋友`)
        } else {
          toast(`已加入房间 ${code}`)
        }
      }),
      EventsOn('room:state', (d: any) => {
        if (d?.operator) {
          if (d.reason === 'forceSeek') {
            toast(`服务器已纠正你的播放进度到 ${fmtTime(d.position)}`, 'warn')
          } else {
            const verb = d.state === 'playing' ? '继续播放' : '暂停了'
            toast(`${d.operator} ${verb} · ${fmtTime(d.position)}`)
          }
        }
      }),
      EventsOn('room:members', (m: any) => {
        if (Array.isArray(m)) setMembers(m)
      }),
      EventsOn('room:notice', (t: string) => toast(String(t))),
      EventsOn('room:error', (e: string) => {
        setBusy(false)
        setError(String(e))
        setConn('disconnected')
      }),
      EventsOn('room:reconnecting', () => setConn('reconnecting')),
      EventsOn('room:closed', () => {
        setConn('reconnecting')
        toast('与服务器的连接断开，正在重连…', 'warn')
      }),
      EventsOn('player:loaded', (v: any) => {
        if (v?.path) setVideo({ path: v.path, name: v.name, size: v.size })
      }),
      EventsOn('player:paused', (p: boolean) => setPaused(Boolean(p))),
      EventsOn('player:exited', () => toast('播放器已退出', 'warn')),
    ]
    return () => offs.forEach((off) => off())
  }, [roomInput, server, toast])

  // 轮询状态：进度显示不依赖事件推送，避免漏事件导致时间不动
  useEffect(() => {
    if (page !== 'player') return
    const t = window.setInterval(async () => {
      try {
        const s: any = await GetState()
        if (!s) return
        if (!scrubbing && seekPending.current === null) setPos(Number(s.position) || 0)
        if (Number(s.duration) > 0) setDur(Number(s.duration))
        setPaused(Boolean(s.paused))
        if (s.conn) setConn(s.conn)
        if (s.room) setRoom(s.room)
        if (Array.isArray(s.members)) setMembers(s.members)
      } catch { /* 播放器尚未就绪 */ }
    }, 1000)
    return () => window.clearInterval(t)
  }, [page, scrubbing])

  // 进播放页时从 mpv 读一次真实音量，之后以本地状态为准
  useEffect(() => {
    if (page !== 'player') return
    GetState().then((s: any) => {
      if (typeof s?.volume === 'number') setVolume(Math.round(s.volume))
    }).catch(() => { })
  }, [page])

  // 全屏：鼠标静止 2 秒后隐藏控制条
  useEffect(() => {
    if (!full) {
      setCtrlVisible(true)
      return
    }
    const wake = () => {
      setCtrlVisible(true)
      window.clearTimeout(hideTimer.current)
      hideTimer.current = window.setTimeout(() => setCtrlVisible(false), 2000)
    }
    wake()
    window.addEventListener('mousemove', wake)
    window.addEventListener('keydown', wake)
    return () => {
      window.removeEventListener('mousemove', wake)
      window.removeEventListener('keydown', wake)
      window.clearTimeout(hideTimer.current)
    }
  }, [full])

  const pickVideo = async () => {
    setError('')
    try {
      const r: any = await ChooseVideo()
      if (!r) return
      setVideo({ path: r.path, name: r.name, size: r.size })
    } catch (e: any) {
      setError(String(e))
    }
  }

  const doCreate = async () => {
    setError('')
    if (!video) { setError('请先选择一部本地视频'); return }
    if (!name.trim()) { setError('请填写昵称'); return }
    if (isLoopback(server)) {
      // 不阻断，但要让人知道：朋友连不上这台机器
      toast('服务器填的是本机回环地址，只有这台电脑能连', 'warn')
    }
    setBusy(true)
    joinAttempt.current = ''
    try {
      await SaveConfig({ server, name, lastVideo: video.path } as any)
      await CreateRoom(name.trim(), password)
    } catch (e: any) {
      setBusy(false)
      setError(String(e))
    }
  }

  const doJoin = async () => {
    setError('')
    if (!roomInput.trim()) { setError('请填写房间码'); return }
    if (!video) { setError('请先选择一部本地视频'); return }
    if (!name.trim()) { setError('请填写昵称'); return }
    if (isLoopback(server)) {
      toast('服务器填的是本机回环地址，朋友连不上这台电脑', 'warn')
    }
    setBusy(true)
    joinAttempt.current = roomInput.trim().toUpperCase()
    try {
      await SaveConfig({ server, name, lastVideo: video.path } as any)
      await JoinRoom(roomInput.trim(), name.trim(), password)
    } catch (e: any) {
      setBusy(false)
      setError(String(e))
    }
  }

  const toggle = async () => {
    try {
      const p = await TogglePlay()
      setPaused(Boolean(p))
    } catch (e: any) { toast(String(e), 'error') }
  }

  const doSeek = async (v: number) => {
    setPos(v)
    try { await Seek(v) } catch (e: any) { toast(String(e), 'error') }
  }

  // 相对跳转：连按方向键时以"上一次的目标"为基准累积，
  // 否则 mpv 还没跳完就去读当前位置，按住右键会原地打转。
  // 连续按键只在停手 260ms 后发一条 seek，服务端那边还有 120ms 防抖。
  const nudgeSeek = (delta: number) => {
    const base = seekPending.current ?? pos
    const max = dur > 0 ? Math.max(0, dur - 0.3) : base + delta
    const target = Math.max(0, Math.min(max, base + delta))
    seekPending.current = target
    setScrubbing(true)
    setPos(target)
    showOsd(`${delta > 0 ? '快进' : '快退'} ${Math.abs(delta)} 秒 · ${fmtTime(target)}`)
    window.clearTimeout(seekTimer.current)
    seekTimer.current = window.setTimeout(() => {
      const v = seekPending.current ?? target
      seekPending.current = null
      setScrubbing(false)
      doSeek(v)
    }, 260)
  }

  const changeRate = async (r: number) => {
    setRate(r)
    try { await SetRate(r) } catch (e: any) { toast(String(e), 'error') }
  }

  const changeVolume = async (v: number) => {
    setVolume(v)
    try { await SetVolume(v) } catch { /* 忽略 */ }
  }

  const nudgeVolume = (delta: number) => {
    const target = Math.max(0, Math.min(100, volume + delta))
    if (target === volume) return
    changeVolume(target)
    showOsd(target === 0 ? '静音' : `音量 ${Math.round(target)}%`)
  }

  const toggleFull = async () => {
    const f = await ToggleFullscreen()
    setFull(Boolean(f))
    window.setTimeout(syncRect, 260)
  }

  // 画面单击 = 播放/暂停；双击 = 全屏。
  // 单击要等 200ms 再执行，否则双击会先触发两次暂停/播放。
  const onSurfaceClick = () => {
    window.clearTimeout(clickTimer.current)
    clickTimer.current = window.setTimeout(() => { toggle() }, 200)
  }

  const onSurfaceDoubleClick = () => {
    window.clearTimeout(clickTimer.current)
    toggleFull()
  }

  const leave = async () => {
    setConfirmLeave(false)
    await LeaveRoom().catch(() => { })
    setPage('home')
    setRoom('')
    setMembers([])
    setConn('idle')
    setPos(0)
    setDur(0)
    setPaused(true)
    setCtrlVisible(true)
  }

  // 播放页键盘快捷键：空格播放/暂停，左右快进快退，上下调音量，
  // F 全屏，M 静音，Esc 关成员浮层/退全屏。
  // 表单页有输入框，所以只在播放页且焦点不在输入框时接管。
  useEffect(() => {
    if (page !== 'player') return
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable)) return
      if (panel === 'logs' || panel === 'settings') return
      if (e.ctrlKey || e.altKey || e.metaKey) return

      const big = e.shiftKey ? 30 : 5
      switch (e.key) {
        case ' ':
        case 'Spacebar':
          e.preventDefault()
          toggle()
          break
        case 'ArrowLeft':
          e.preventDefault()
          nudgeSeek(-big)
          break
        case 'ArrowRight':
          e.preventDefault()
          nudgeSeek(big)
          break
        case 'ArrowUp':
          e.preventDefault()
          nudgeVolume(5)
          break
        case 'ArrowDown':
          e.preventDefault()
          nudgeVolume(-5)
          break
        case 'f':
        case 'F':
          e.preventDefault()
          toggleFull()
          break
        case 'm':
        case 'M':
          e.preventDefault()
          changeVolume(volume > 0 ? 0 : 100)
          showOsd(volume > 0 ? '静音' : '音量 100%')
          break
        case 'Escape':
          if (panel === 'members') setPanel(null)
          else if (full) toggleFull()
          else if (confirmLeave) setConfirmLeave(false)
          break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [page, panel, full, pos, dur, volume, confirmLeave])

  const openLogs = async () => {
    setLogs(await GetLogs().catch(() => ''))
    setPanel('logs')
  }

  const checkServer = async () => {
    setServerCheck('检测中…')
    try {
      const r: any = await PingServer(server)
      setServerCheck(r?.message || (r?.ok ? '连接正常' : '连接失败'))
    } catch (e: any) {
      setServerCheck(String(e))
    }
  }

  const otherDur = useMemo(() => {
    return members.some((m) => m.duration && dur > 0 && Math.abs(m.duration - dur) > 2)
  }, [members, dur])

  const noticeBar = (
    <div className="notices">
      {notices.map((n) => (
        <div key={n.id} className={`notice ${n.kind}`}>
          {n.kind === 'info' ? <IconCheck size={16} /> : <IconAlert size={16} />}
          <span>{n.text}</span>
        </div>
      ))}
    </div>
  )

  if (page === 'home') {
    return (
      <div className="app">
        <TopBar onSettings={() => setPanel('settings')} onLogs={openLogs} />
        <div className="home">
          <div className="hero">
            <h1>和朋友，看同一时刻。</h1>
            <p>本地视频不上传，只同步播放控制。</p>
          </div>

          <label className="field narrow">
            <span className="label">昵称</span>
            <input className="input" value={name} placeholder="房间里显示的名字"
              onChange={(e) => setName(e.target.value)} maxLength={16} />
          </label>

          <div className="entry-row">
            <button className="entry-card" onClick={() => { setError(''); setPage('create') }}>
              <IconPlus size={22} />
              <span className="entry-title">创建房间</span>
              <span className="entry-sub">选择视频并邀请朋友</span>
            </button>
            <button className="entry-card" onClick={() => { setError(''); setPage('join') }}>
              <IconArrowRight size={22} />
              <span className="entry-title">加入房间</span>
              <span className="entry-sub">输入朋友的房间码</span>
            </button>
          </div>

          <div className="server-line">
            <span className={server && !isLoopback(server) ? 'ok' : 'warn'}>
              {server ? `服务器 ${server}` : '未配置服务器地址'}
            </span>
            <button className="link" onClick={() => setPanel('settings')}>高级设置</button>
          </div>
          {isLoopback(server) && (
            <div className="error-line">
              这是本机回环地址：只有这台电脑能连，朋友永远接不上。请改成服务器的公网地址（形如 1.2.3.4:8995）。
            </div>
          )}
          {error && <div className="error-line">{error}</div>}
        </div>
        {noticeBar}
        {panel === 'settings' && (
          <SettingsPanel
            server={server} setServer={setServer} name={name} setName={setName}
            check={serverCheck} onCheck={checkServer} version={version}
            onClose={() => setPanel(null)}
            onSave={async () => { await SaveConfig({ server, name } as any); setPanel(null); toast('设置已保存') }}
          />
        )}
        {panel === 'logs' && <LogsPanel logs={logs} onClose={() => setPanel(null)} onClear={async () => { await ClearLogs(); setLogs(''); toast('日志已清空') }} />}
      </div>
    )
  }

  if (page === 'create' || page === 'join') {
    const isJoin = page === 'join'
    return (
      <div className="app">
        <TopBar onSettings={() => setPanel('settings')} onLogs={openLogs} />
        <div className="form-page">
          <button className="back" onClick={() => { setPage('home'); setError('') }}>
            <IconBack size={18} /> 返回
          </button>
          <h2>{isJoin ? '加入房间' : '创建房间'}</h2>

          {isJoin && (
            <label className="field">
              <span className="label">房间码 <em>*</em></span>
              <input
                className="input code-input"
                value={roomInput}
                placeholder="例如 6ZJ46H"
                onChange={(e) => setRoomInput(e.target.value.toUpperCase().replace(/\s/g, ''))}
                maxLength={8}
              />
            </label>
          )}

          <div className="field">
            <span className="label">本地视频 <em>*</em></span>
            <button className={`dropzone ${video ? 'has-file' : ''}`} onClick={pickVideo}>
              {video ? (
                <>
                  <IconFilm size={20} />
                  <span className="file-name">{video.name}</span>
                  <span className="file-meta">{fmtSize(video.size)}</span>
                  <span className="file-change">重新选择</span>
                </>
              ) : (
                <>
                  <IconFilm size={22} />
                  <span>点击选择视频文件</span>
                  <span className="dim">支持 mpv 可播放的本地格式</span>
                </>
              )}
            </button>
          </div>

          <label className="field">
            <span className="label">昵称 <em>*</em></span>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} maxLength={16} />
          </label>

          <label className="field">
            <span className="label">房间密码 <span className="dim">（可选）</span></span>
            <input className="input" value={password} placeholder="不填则凭房间码即可进入"
              onChange={(e) => setPassword(e.target.value)} maxLength={16} />
          </label>

          {error && <div className="error-box"><IconAlert size={16} />{error}</div>}

          <div className="form-actions">
            <button className="btn primary lg" disabled={busy} onClick={isJoin ? doJoin : doCreate}>
              {busy ? '连接中…' : isJoin ? '加入并同步' : '创建并开始观看'}
            </button>
          </div>
        </div>
        {noticeBar}
        {panel === 'settings' && (
          <SettingsPanel
            server={server} setServer={setServer} name={name} setName={setName}
            check={serverCheck} onCheck={checkServer} version={version}
            onClose={() => setPanel(null)}
            onSave={async () => { await SaveConfig({ server, name } as any); setPanel(null); toast('设置已保存') }}
          />
        )}
        {panel === 'logs' && <LogsPanel logs={logs} onClose={() => setPanel(null)} onClear={async () => { await ClearLogs(); setLogs(''); toast('日志已清空') }} />}
      </div>
    )
  }

  const showPanel = panel === 'members'
  const connLabel = conn === 'connected' ? '已连接' : conn === 'reconnecting' ? '重连中…' : conn === 'connecting' ? '连接中' : '已断开'
  return (
    <div className={`app player ${full ? 'is-full' : ''} ${ctrlVisible ? '' : 'hide-ctrl'}`}>
      <div className={`stage ${showPanel ? 'with-panel' : ''}`}>
        <div className="video-wrap clickable" ref={videoRef}
          onClick={onSurfaceClick}
          onDoubleClick={onSurfaceDoubleClick}>
          <div className="video-area" />
        </div>

        {osd && <div className="osd mono">{osd}</div>}

        <div className="overlay top">
          <div className="title-line">
            <span className="film-name">{video?.name || '本地视频'}</span>
            <button className="room-chip" onClick={() => { CopyText(room); toast(`房间码 ${room} 已复制`) }}>
              <span className="mono">{room}</span>
              <IconCopy size={14} />
            </button>
            <span className={`conn-chip ${conn}`} title={server ? `服务器 ${server}` : '未配置服务器'}>
              <i /> {connLabel}
            </span>
            {server && (
              <span className={`srv-chip mono ${isLoopback(server) ? 'warn' : ''}`}
                title={isLoopback(server) ? '这是本机回环地址，朋友连不上；请改成公网地址' : '当前服务器地址'}>
                {isLoopback(server) ? '仅本机 · ' : ''}{server}
              </span>
            )}
            <span className="dur-chip mono">{fmtTime(pos)} / {fmtTime(dur)}</span>
          </div>
          <div className="top-actions">
            <button className={`icon-btn ${otherDur ? 'has-warn' : ''}`}
              title={otherDur ? '有成员片源时长不一致' : '成员'}
              onClick={() => setPanel(showPanel ? null : 'members')}>
              <IconUsers size={18} />
              {otherDur && <i className="warn-dot" />}
            </button>
            <button className="icon-btn" title={full ? '退出全屏' : '全屏'} onClick={toggleFull}>
              {full ? <IconExitFullscreen size={18} /> : <IconFullscreen size={18} />}
            </button>
          </div>
        </div>

        {showPanel && (
          <aside className="members">
            <div className="members-head">
              <span>房间成员</span>
              <span className="count">{members.length}/5</span>
              <button className="icon-btn" onClick={() => setPanel(null)}><IconClose size={16} /></button>
            </div>
            <div className="members-body">
              {members.length === 0 && <div className="empty">还没有成员信息</div>}
              {members.map((m) => (
                <div className="member" key={m.name}>
                  <div className="member-row">
                    <span className={`dot ${stateDot(m.state)}`} />
                    <span className="member-name">{m.name}</span>
                  </div>
                  <div className="member-meta">
                    {stateCn(m.state)}{m.duration ? ` · ${fmtTime(m.duration)}` : ''}
                  </div>
                  {m.duration && dur > 0 && Math.abs(m.duration - dur) > 2 && (
                    <div className="member-warn"><IconAlert size={13} /> 时长与你不一致</div>
                  )}
                </div>
              ))}
            </div>
            {otherDur && <div className="members-tip">有成员片源时长不一致，可能越看越歪</div>}
            <div className="members-foot">
              <span className="dim small">服务器</span>
              <span className={`mono small ${isLoopback(server) ? 'warn-text' : ''}`}>{server || '未配置'}</span>
              {isLoopback(server) && <span className="members-tip danger">回环地址只有本机能连，朋友连不上</span>}
            </div>
          </aside>
        )}

        <div className="overlay bottom">
          {error && (
            <div className="blocking">
              <IconAlert size={18} />
              <span>{error}</span>
              <button className="btn sm" onClick={() => setError('')}>知道了</button>
            </div>
          )}
          <div className="ctrlbar">
            <button className="play-btn" onClick={toggle} title={paused ? '播放' : '暂停'}>
              {paused ? <IconPlay size={20} /> : <IconPause size={20} />}
            </button>
            <span className="time mono">{fmtTime(pos)}</span>
            <input
              className="scrub" type="range" min={0} max={dur > 0 ? dur : 1} step={0.2}
              value={Math.min(pos, dur || 1)}
              onChange={(e) => { setScrubbing(true); setPos(Number(e.currentTarget.value)) }}
              onPointerUp={(e) => {
                // 直接用控件自己的值，不依赖上一次 setPos 的渲染结果
                const v = Number(e.currentTarget.value)
                setScrubbing(false)
                doSeek(v)
                e.currentTarget.blur() // 拖完交还焦点，空格/方向键继续走全局快捷键
              }}
              onKeyUp={(e) => { if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') doSeek(Number(e.currentTarget.value)) }}
            />
            <span className="time mono dim">{fmtTime(dur)}</span>

            <div className="rate-group">
              {[0.5, 1, 1.5, 2].map((r) => (
                <button key={r} className={`chip ${rate === r ? 'on' : ''}`} onClick={() => changeRate(r)}>
                  {r}×
                </button>
              ))}
            </div>

            <div className="vol">
              <button className="icon-btn" title="静音 (M)" onClick={() => nudgeVolume(volume > 0 ? -volume : 100)}>
                {volume > 0 ? <IconVolume size={18} /> : <IconMute size={18} />}
              </button>
              <input className="vol-slider" type="range" min={0} max={100} value={volume}
                onChange={(e) => changeVolume(Number(e.currentTarget.value))}
                onPointerUp={(e) => e.currentTarget.blur()} />
            </div>

            <button className="btn ghost sm" onClick={() => setConfirmLeave(true)}>离开</button>
          </div>
        </div>

      </div>

      {noticeBar}

      {joinWarn && (
        <div className="modal-mask">
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h3>没有加入朋友的房间</h3>
            <p>
              房间码 <b className="mono">{joinWarn.room}</b> 在服务器 <b className="mono">{joinWarn.server}</b> 上并不存在，
              服务器替你新建了一个同码的空房间。所以现在房间里只有你一个人，朋友的操作你也收不到。
            </p>
            <p className="hint-line">常见原因：① 房间码打错或已被回收（空房 15 分钟后失效）；② 你和朋友填的服务器地址不是同一台。</p>
            <div className="modal-actions">
              <button className="btn" onClick={() => setJoinWarn(null)}>我知道，先留在这里</button>
              <button className="btn primary" onClick={async () => {
                setJoinWarn(null)
                await LeaveRoom().catch(() => { })
                setPage('join')
                setRoom('')
                setMembers([])
                setConn('idle')
              }}>返回重新输入</button>
            </div>
          </div>
        </div>
      )}

      {confirmLeave && (
        <div className="modal-mask" onClick={() => setConfirmLeave(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h3>离开房间？</h3>
            <p>只退出当前房间，不会删除本地视频。</p>
            <div className="modal-actions">
              <button className="btn" onClick={() => setConfirmLeave(false)}>继续看</button>
              <button className="btn danger" onClick={leave}>离开</button>
            </div>
          </div>
        </div>
      )}

      {panel === 'logs' && <LogsPanel logs={logs} onClose={() => setPanel(null)} onClear={async () => { await ClearLogs(); setLogs(''); toast('日志已清空') }} />}
    </div>
  )
}

// ---------- 通用组件 ----------

function TopBar({ onSettings, onLogs }: { onSettings: () => void; onLogs: () => void }) {
  return (
    <header className="topbar">
      <div className="brand"><span className="brand-dot" /> 一起看 · SyncWatch</div>
      <div className="topbar-actions">
        <button className="icon-btn" title="设置" onClick={onSettings}><IconSettings size={18} /></button>
        <button className="icon-btn" title="运行日志" onClick={onLogs}><IconLog size={18} /></button>
      </div>
    </header>
  )
}

function SettingsPanel(props: {
  server: string; setServer: (s: string) => void
  name: string; setName: (s: string) => void
  check: string; onCheck: () => void
  version?: string
  onClose: () => void; onSave: () => void
}) {
  const loopback = isLoopback(props.server)
  return (
    <div className="drawer-mask" onClick={props.onClose}>
      <div className="drawer" onClick={(e) => e.stopPropagation()}>
        <div className="drawer-head">
          <span>设置</span>
          <button className="icon-btn" onClick={props.onClose}><IconClose size={16} /></button>
        </div>
        <div className="drawer-body">
          <label className="field">
            <span className="label">服务器地址</span>
            <input className={`input mono ${loopback ? 'input-warn' : ''}`} value={props.server} placeholder="例如 1.2.3.4:8995"
              onChange={(e) => props.setServer(e.target.value)} />
          </label>
          <div className="row-between">
            <span className="dim small">填你自己的同步服务器，端口默认 8995</span>
            <button className="btn sm" onClick={props.onCheck}>测试连接</button>
          </div>
          {props.check && <div className="hint">{props.check}</div>}
          {loopback && (
            <div className="error-box">
              <IconAlert size={16} />
              <span>
                <b>{props.server}</b> 是本机回环地址，只有这台电脑能连。
                和朋友一起看时，必须填服务器的公网地址（形如 <code>1.2.3.4:8995</code>），
                否则你俩各自连着自己的服务器，界面都显示"已连接"，但互相看不到、操作也传不过去。
              </span>
            </div>
          )}

          <label className="field">
            <span className="label">默认昵称</span>
            <input className="input" value={props.name} onChange={(e) => props.setName(e.target.value)} maxLength={16} />
          </label>

          <div className="note">
            <IconLink size={15} />
            <span>播放器使用程序目录下的 <code>mpv\mpv.exe</code>；找不到时可用环境变量 <code>SYNCWATCH_MPV</code> 指定。</span>
          </div>

          <div className="note">
            <IconLink size={15} />
            <span>快捷键：空格播放/暂停 · ←/→ 快退快进 5 秒（按住 Shift 为 30 秒）· ↑/↓ 音量 · F 全屏 · M 静音。单击画面也能暂停/播放，双击全屏。</span>
          </div>
        </div>
        <div className="drawer-foot">
          <span className="dim small">{props.version}</span>
          <button className="btn primary" onClick={props.onSave}>保存</button>
        </div>
      </div>
    </div>
  )
}

function LogsPanel({ logs, onClose, onClear }: { logs: string; onClose: () => void; onClear: () => void }) {
  return (
    <div className="drawer-mask" onClick={onClose}>
      <div className="drawer wide" onClick={(e) => e.stopPropagation()}>
        <div className="drawer-head">
          <span>运行日志</span>
          <div className="row-gap">
            <button className="btn sm" onClick={() => navigator.clipboard?.writeText(logs)}>复制</button>
            <button className="btn sm" onClick={onClear}>清空</button>
            <button className="icon-btn" onClick={onClose}><IconClose size={16} /></button>
          </div>
        </div>
        <pre className="logs">{logs || '暂无日志'}</pre>
      </div>
    </div>
  )
}
