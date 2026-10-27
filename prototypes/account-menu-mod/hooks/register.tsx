import type { Register } from 'claude-code'

const PANE = 'account-menu'

type Json = null | boolean | number | string | Json[] | { [k: string]: Json }

// Run `cid` by argv (no shell) and parse its JSON; null when it fails.
async function cidJson($: any, argv: string[]): Promise<Json> {
  try {
    const r = await $.process.run(['cid', ...argv])
    if (r.exitCode !== 0) return null
    return JSON.parse(r.stdout) as Json
  } catch {
    return null
  }
}

function resetsIn(iso: string | undefined, now: number): string {
  if (!iso) return ''
  const ms = Date.parse(iso) - now
  if (!(ms > 0)) return 'Resets soon'
  const m = Math.round(ms / 60000)
  const d = Math.floor(m / 1440)
  const h = Math.floor((m % 1440) / 60)
  const mm = m % 60
  return 'Resets in ' + (d > 0 ? `${d}d ${h}h` : h > 0 ? `${h}h ${mm}m` : `${mm}m`)
}

// One snapshot of everything the view draws; the CLI is the source of truth.
async function load($: any, error: string | null = null) {
  const accounts = (await cidJson($, ['list', '--json'])) ?? []
  const config = (await cidJson($, ['config', '--json'])) as { settings?: Json } | null
  let limits: Json[] = []
  try {
    const now = await $.clock.now()
    const usage = await $.session.usage()
    limits = (usage.rateLimits as any[])
      .filter(l => l.kind === 'five_hour' || l.kind === 'seven_day')
      .map(l => ({
        kind: l.kind,
        percentUsed: l.percentUsed,
        resets: resetsIn(l.resetsAt, now),
      }))
  } catch {
    // usage is optional
  }
  return {
    rev: Date.now(),
    accounts,
    settings: config?.settings ?? [],
    limits,
    error,
  } as Record<string, Json>
}

async function openMenu($: any) {
  await $.ui.open({ id: PANE, title: 'Account', focus: true, closeOnEscape: true })
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({
      name: 'account-menu',
      description: 'Open the interactive account menu',
    })
    return next(e)
  })

  on('command.run', { command: 'account-menu' }, async $ => {
    await openMenu($)
    return { text: 'Account menu opened.' }
  })

  // /account with no arguments opens the menu; any argument reaches the
  // existing account-switch skill and its UserPromptExpansion hook untouched.
  for (const name of ['account', 'account-switch:account']) {
    on('command.run', { command: name }, async ($, e, next) => {
      if (e.args.trim() !== '') return next(e)
      await openMenu($)
      return { text: '' }
    }).catch(($, e, next) => next(e)) // on failure the skill runs as before
  }

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e, next) => {
    // vscode and mobile have no Client; leave their drawing to the engine.
    if (e.surface !== 'terminal' && e.surface !== 'desktop') return next(e)
    const { Client } = $.ui.resolve(e)
    const data = await load($)
    return <Client key="menu" module="./menu.tsx" props={data} />
  })

  on('ui.message', async ($, e) => {
    if (e.element !== 'menu') return {}
    const d = e.data as { type?: string; name?: string; key?: string; value?: string }
    let error: string | null = null
    if (d.type === 'switch' && d.name) {
      const r = await $.process.run(['cid', 'switch', d.name])
      if (r.exitCode !== 0) error = (r.stderr || r.stdout).trim().slice(0, 200)
    } else if (d.type === 'set' && d.key && d.value !== undefined) {
      const r = await $.process.run(['cid', 'config', 'set', d.key, d.value])
      if (r.exitCode !== 0) error = (r.stderr || r.stdout).trim().slice(0, 200)
    }
    return { props: await load($, error) }
  })
}
