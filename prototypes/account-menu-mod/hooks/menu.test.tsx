import { expect, test } from 'claude-code/testing'

const PANE = {
  plugin: 'account-menu',
  component: 'Pane',
  requestId: 'account-menu',
  props: {
    title: 'Account',
    isFocused: true,
    bodyColumns: 80,
    placement: 'dock',
    scroll: { offset: 0, bodyRows: 20 },
    view: {},
  },
} as const

const LIST = [
  { index: 1, name: 'nico', email: 'nico@example.com', active: true, status: 'ok' },
  { index: 2, name: 'luna', email: 'luna@example.com', active: false, status: 'ok' },
]
const CONFIG = {
  paths: {},
  settings: [
    { key: 'language', value: 'auto', source: 'default', default: 'auto', kind: 'enum', allowed: ['auto', 'en', 'es'], env: 'CID_LANG', description: 'x' },
  ],
}

async function has(ui: any, re: RegExp, surface: string) {
  const all = (await ui.findAll({ type: 'Text', in: 'menu' })).map((t: any) => t.text)
  const ok = all.some((t: string) => re.test(t))
  if (!ok) throw new Error(`missing ${re} on ${surface}: ${JSON.stringify(all)}`)
  return ok
}

test('list renders, right opens detail, left returns, tab switches', async ($, on) => {
  const calls: string[][] = []
  on('process.run', (_, e: any) => {
    calls.push(e.argv ?? e)
    const isList = JSON.stringify(e).includes('list')
    return { value: { exitCode: 0, stdout: JSON.stringify(isList ? LIST : CONFIG), stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('clock.now', () => ({ value: Date.now() }))
  on('session.usage', () => ({
    value: {
      startedAt: 0,
      context: {} as any,
      rateLimits: [{ kind: 'five_hour', percentUsed: 42, resetsAt: new Date(Date.now() + 3600e3).toISOString() }],
    },
  }))

  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ ...PANE, surface, viewport: { columns: 80, rows: 24 } })
    expect(await has(ui, /> nico/, surface)).toBe(true)
    expect(await has(ui, /luna/, surface)).toBe(true)

    await ui.key({ key: 'down', in: 'menu' })
    expect(await has(ui, /> luna/, surface)).toBe(true)
    await ui.key({ key: 'up', in: 'menu' })
    await ui.key({ key: 'right', in: 'menu' })
    expect(await has(ui, /5h/, surface)).toBe(true)
    expect(await has(ui, /42% used/, surface)).toBe(true)
    await ui.key({ key: 'left', in: 'menu' })
    expect(await has(ui, /> nico/, surface)).toBe(true)

    await ui.key({ key: 'tab', in: 'menu' })
    expect(await has(ui, /language/, surface)).toBe(true)
    expect(await has(ui, /auto ›/, surface)).toBe(true)
    await ui.unmount()
  }
  // Rendering only reads: no switch / set was ever run.
  expect(JSON.stringify(calls)).not.toContain('switch')
  expect(JSON.stringify(calls)).not.toContain('"set"')
})

const TYPED = {
  origin: { kind: 'composer' },
  presentation: { isFullscreen: false, columns: 80 },
} as const

test('/account with no args opens the pane; with args it reaches the skill', async ($, on) => {
  const opened: unknown[] = []
  on('ui.open', (_, e) => (opened.push(e), { value: { isPlaced: true } }))
  // Stands for the engine + the account-switch skill beneath the mod.
  on('command.run', (_, e) => ({ text: `skill ran: ${e.command} ${e.args}` }))

  const bare = await $.command.run({ command: 'account', args: '', ...TYPED })
  expect(opened.length).toBe(1)
  expect(bare.text).toBe('')

  const withArgs = await $.command.run({ command: 'account', args: 'list', ...TYPED })
  expect(opened.length).toBe(1)
  expect(withArgs.text).toBe('skill ran: account list')
})

test('enter on an inactive account runs cid switch (fake) and refreshes', async ($, on) => {
  const argvs: string[] = []
  let active = 'nico'
  on('clock.now', () => ({ value: Date.now() }))
  on('session.usage', () => ({ value: { startedAt: 0, context: {} as any, rateLimits: [] } }))
  on('process.run', (_, e: any) => {
    const argv: string[] = e.argv ?? e
    argvs.push(argv.join(' '))
    if (argv[1] === 'switch') active = argv[2] as string
    const out = argv[1] === 'config' ? CONFIG : LIST.map(a => ({ ...a, active: a.name === active }))
    return { value: { exitCode: 0, stdout: JSON.stringify(out), stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal', viewport: { columns: 80, rows: 24 } })
  await ui.key({ key: 'down', in: 'menu' })
  await ui.key({ key: 'return', in: 'menu' })
  expect(argvs).toContain('cid switch luna')
  expect(await has(ui, /luna.*|active/, 'terminal')).toBe(true)
  await ui.unmount()
})
