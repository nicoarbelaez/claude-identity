import type { ClientKeyEvent, ClientModule, ClientElements, ThemeKey } from 'claude-code'

type Account = { index: number; name: string; email: string; active: boolean; status: string }
type Setting = {
  key: string
  value: string
  source: string
  default: string
  kind: string
  allowed?: string[]
  env?: string
  description?: string
}
type Limit = { kind: string; percentUsed: number; resets: string }
type Data = {
  rev: number
  accounts: Account[]
  settings: Setting[]
  limits: Limit[]
  error: string | null
}
type State = { tab: number; cursor: number[]; detail: boolean; waitingRev: number | null }
type Ctx = { el: ClientElements; data: Data; st: State }
type Action = { type: 'switch'; name: string } | { type: 'set'; key: string; value: string }

// ---- Tab registry: add a tab by adding one entry. -------------------------
type TabDef = {
  id: string
  title: string
  size: (d: Data) => number
  hasDetail: boolean
  list: (c: Ctx) => unknown
  detail?: (c: Ctx) => unknown
  // Enter on the selected row; returns the CLI action to post, if any.
  enter: (d: Data, i: number) => Action | null
}

const TABS: TabDef[] = [
  {
    id: 'accounts',
    title: 'Accounts',
    size: d => d.accounts.length,
    hasDetail: true,
    list: c =>
      c.data.accounts.map((a, i) =>
        Row(
          c,
          i === (c.st.cursor[0] ?? 0),
          `${a.name}  ${a.email}`,
          a.active ? 'active' : a.status === 'ok' ? '' : a.status,
        ),
      ),
    detail: AccountDetail,
    enter: (d, i) => {
      const a = d.accounts[i]
      return a && !a.active ? { type: 'switch', name: a.name } : null
    },
  },
  {
    id: 'config',
    title: 'Config',
    size: d => d.settings.length,
    hasDetail: false,
    list: c =>
      c.data.settings.map((s, i) =>
        Row(c, i === (c.st.cursor[1] ?? 0), s.key, s.value, s.kind === 'enum'),
      ),
    enter: (d, i) => {
      const s = d.settings[i]
      if (!s || s.kind !== 'enum' || !s.allowed?.length) return null
      const at = s.allowed.indexOf(s.value)
      return { type: 'set', key: s.key, value: s.allowed[(at + 1) % s.allowed.length] as string }
    },
  },
]

// ---- Shared pieces: Tabs, Row, Bar, Hint. ---------------------------------
function Tabs(c: Ctx) {
  const { Box, Text } = c.el
  return (
    <Box>
      {TABS.map((t, i) =>
        i === c.st.tab ? (
          <Text inverse bold>{` ${t.title} `}</Text>
        ) : (
          <Text dimColor>{` ${t.title} `}</Text>
        ),
      )}
    </Box>
  )
}

function Row(c: Ctx, selected: boolean, label: string, value: string, chevron = false) {
  const { Box, Text } = c.el
  const color: ThemeKey | undefined = selected ? 'suggestion' : undefined
  return (
    <Box justifyContent="space-between">
      <Text color={color}>{(selected ? '> ' : '  ') + label}</Text>
      <Text color={color} dimColor={!selected}>{value + (chevron ? ' ›' : '')}</Text>
    </Box>
  )
}

function Bar(c: Ctx, title: string, l: Limit) {
  const { Box, Text } = c.el
  const width = 20
  const filled = Math.max(0, Math.min(width, Math.round((l.percentUsed / 100) * width)))
  const color: ThemeKey = l.percentUsed >= 90 ? 'error' : l.percentUsed >= 70 ? 'warning' : 'success'
  return (
    <Box flexDirection="column">
      <Text bold>{title}</Text>
      <Box>
        <Text color={color}>{'█'.repeat(filled)}</Text>
        <Text dimColor>{'░'.repeat(width - filled)}</Text>
        <Text>{` ${Math.round(l.percentUsed)}% used`}</Text>
      </Box>
      {l.resets ? <Text dimColor>{l.resets}</Text> : null}
    </Box>
  )
}

function Hint(c: Ctx, text: string) {
  const { Text } = c.el
  return <Text dimColor>{text}</Text>
}

function AccountDetail(c: Ctx) {
  const { Box, Text } = c.el
  const a = c.data.accounts[c.st.cursor[0] ?? 0]
  if (!a) return <Text dimColor>No account.</Text>
  const five = c.data.limits.find(l => l.kind === 'five_hour')
  const week = c.data.limits.find(l => l.kind === 'seven_day')
  return (
    <Box flexDirection="column" gap={1}>
      <Box flexDirection="column">
        <Text bold>{a.name}</Text>
        <Text dimColor>{a.email}</Text>
        <Text>{`Status: ${a.status}${a.active ? ' (active)' : ''}`}</Text>
      </Box>
      {a.active && five ? Bar(c, 'Current session (5h)', five) : null}
      {a.active && week ? Bar(c, 'Current week', week) : null}
      <Box flexDirection="column">
        <Text color={a.active ? undefined : 'suggestion'} dimColor={a.active}>
          {a.active ? 'Switch (already active)' : 'Enter: switch to this account'}
        </Text>
        <Text dimColor>Relogin (not in the spike)</Text>
        <Text dimColor>Remove (not in the spike)</Text>
      </Box>
    </Box>
  )
}

// ---- The surface module. ---------------------------------------------------
// The key listener must read the freshest props and state even when several
// keys arrive before the next frame, so both live in a module-level ref that
// every render and every key updates.
const live: { data: Data; st: State; set: (s: State) => void; post: (a: Action) => void } = {
  data: { rev: 0, accounts: [], settings: [], limits: [], error: null },
  st: { tab: 0, cursor: [0, 0], detail: false, waitingRev: null },
  set: () => {},
  post: () => {},
}

function onKey(e: ClientKeyEvent) {
  const { data } = live
  let st = live.st
  const tab = TABS[st.tab] as TabDef
  const move = (n: number) => {
    const cursor = st.cursor.slice()
    const max = Math.max(0, tab.size(data) - 1)
    cursor[st.tab] = Math.max(0, Math.min(max, (cursor[st.tab] ?? 0) + n))
    st = { ...st, cursor }
  }
  switch (e.key) {
    case 'up':
      if (!st.detail) move(-1)
      break
    case 'down':
      if (!st.detail) move(1)
      break
    case 'right':
      if (tab.hasDetail && tab.size(data) > 0) st = { ...st, detail: true }
      break
    case 'left':
      st = { ...st, detail: false }
      break
    case 'tab': {
      const next = (st.tab + (e.shift ? TABS.length - 1 : 1)) % TABS.length
      st = { ...st, tab: next, detail: false }
      break
    }
    case 'return': {
      if (st.waitingRev !== null) break
      const action = tab.enter(data, st.cursor[st.tab] ?? 0)
      if (action) {
        st = { ...st, waitingRev: data.rev }
        live.post(action)
      }
      break
    }
    default:
      return
  }
  live.st = st
  live.set(st)
}

const AccountMenu: ClientModule<any, State> = (props, surface) => {
  const data = props as Data
  const st: State = surface.state ?? { tab: 0, cursor: [0, 0], detail: false, waitingRev: null }
  // A fresh snapshot (new rev) ends the wait for the action in flight.
  const fresh = st.waitingRev !== null && data.rev !== st.waitingRev ? { ...st, waitingRev: null } : st
  live.data = data
  live.st = fresh
  live.set = surface.setState
  live.post = a => surface.post(a)
  surface.onKey(onKey)

  const c: Ctx = { el: surface.elements, data, st: fresh }
  const { Box, Text } = surface.elements
  const tab = TABS[fresh.tab] as TabDef
  const body = fresh.detail && tab.detail ? tab.detail(c) : tab.list(c)
  const hint = fresh.detail
    ? '← back · enter select · esc close'
    : '↑↓ move · → details · tab switch tab · enter select · esc close'

  return (
    <Box flexDirection="column" gap={1}>
      {Tabs(c)}
      <Box flexDirection="column">{body as any}</Box>
      {data.error ? <Text color="error">{data.error}</Text> : null}
      {fresh.waitingRev !== null ? <Text dimColor>Working...</Text> : null}
      {Hint(c, hint)}
    </Box>
  )
}

export default AccountMenu
