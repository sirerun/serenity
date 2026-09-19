#!/usr/bin/env python3
"""Validate/render the static E23 completion plan; never dispatch or mutate services."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PLAN = ROOT / 'docs/launch/hosted-completion/tasks.json'
CONTRACTS = ROOT / 'docs/tasks/hosted-completion'

def dependencies(task, profile):
    return set(task['dependencies'] + task['profile_dependencies'].get(profile, []))

def render(task):
    # From docs/tasks/hosted-completion, two levels return to docs.
    link = '../../launch/hosted-plan.md'
    lines = [f"# {task['id']} — {task['title']}", '',
        '> Generated from `docs/launch/hosted-completion/tasks.json`; coordinator edits the registry and runs `--render`.', '',
        f"Read the [execution protocol]({link}) and [worker prompt](../../launch/hosted-completion/worker-prompt.md) first.", '',
        f"- Lane: **{task['lane']}**; executor: Claude Code Sonnet, except named human/technical gates.",
        f"- Planning state: `{task['status']}` (not proof of completion).",
        f"- Profiles: {', '.join(task['profiles'])}.",
        f"- Dependencies: {', '.join(task['dependencies']) or 'none'}.",
        f"- Profile dependencies: {json.dumps(task['profile_dependencies'], sort_keys=True)}.",
        f"- External gates: {', '.join(task['external_gates']) or 'none'}.",
        f"- File resource claims: {', '.join(task['resource_claims']) or 'task claim only'}.",
        f"- Additional live-phase claims: {', '.join(task['live_resource_claims']) or 'none'}.",
        f"- Estimate: {task['estimate_agent_hours']} active agent-hours; excludes external waits.",
        f"- Original requirements: {', '.join(task['original_requirements'])}.",
        f"- Evidence: `{task['evidence']}` plus adjacent sanitized artifacts.", '',
        '## Exclusive write scope', '',
        'Claim this task and the corresponding file resources before edits. Other paths are read-only; submit a precise integration request instead of editing shared files.', '']
    lines += [f'- `{p}`' for p in task['write_paths']]
    lines += ['', 'The task also owns its evidence directory. No worker edits the task registry, aggregate status/acceptance, or another task’s receipts.', '', '## Required steps', '']
    lines += [f'{i}. {s}' for i, s in enumerate(task['steps'], 1)]
    lines += ['', '## Acceptance — all required', '']
    lines += [f'- [ ] {s}' for s in task['acceptance']]
    if task.get('allowed_dependency_exceptions'):
        lines += ['', 'Only permitted dependency exceptions:', '']
        lines += [f'- {k}: {v}.' for k,v in task['allowed_dependency_exceptions'].items()]
    lines += ['', '## Verification commands', '',
        'Run from your isolated repository root. New harness paths/flags below are implementation deliverables, not commands claimed to exist today. Resolve `$QUALIFICATION_MANIFEST`, `$EVIDENCE_DIR` and any binary path before running; record the fully resolved command. A `--help` invocation is never acceptance evidence. Follow the build-lease protocol for multi-package Go work.', '', '```sh']
    lines += task['commands']
    lines += ['```', '', 'For an external gate, prepare/test fixtures now but execute the live command only when that gate’s evidence is recorded. Missing credentials result in BLOCKED for the live phase, not PASS or a silent skip.', '', '## Handoff / stop conditions', '',
        'Open one scoped PR with the task ID, source SHA, tests/counts, evidence level, known limits and integration requests. Hold on changed interfaces, scope expansion, unexpected cost or a trusted review hold. Fix failures in scope; otherwise record BLOCKED with the exact owner/input required. Do not merge your own PR or deploy because CI is green. The coordinator accepts only after the dependency revision, review and relevant CI are verified.', '']
    return '\n'.join(lines)

def overlap(a,b):
    a=a.removesuffix('/**'); b=b.removesuffix('/**')
    return a==b or a.startswith(b+'/') or b.startswith(a+'/')

def validate(plan):
    tasks=plan['tasks']; index={t['id']:t for t in tasks}
    assert len(index)==len(tasks), 'duplicate task IDs'
    assert len(plan['baseline'])==40, 'baseline must be full SHA'
    original={f'T23.{n}' for n in range(1,41)}
    assert original == {x for t in tasks for x in t['original_requirements']}, 'original E23 coverage gap'
    for t in tasks:
        for field in ('title','lane','steps','acceptance','commands','write_paths','evidence'):
            assert t[field], f"{t['id']}: empty {field}"
        assert t['evidence']==f"docs/launch/evidence/{t['id']}/result.json"
        for path in t['write_paths']:
            assert not path.startswith('/') and '..' not in Path(path).parts, f'unsafe scope {path}'
        assert set(t['profiles']) <= set(plan['profiles'])
        for ds in [t['dependencies'],*t['profile_dependencies'].values()]:
            assert len(ds)==len(set(ds)),f"{t['id']}: duplicate dependencies"
        for profile in t['profiles']:
            for dep in dependencies(t,profile):
                assert dep in index and profile in index[dep]['profiles'], f"{t['id']}: invalid {profile} dependency {dep}"
    for profile in plan['profiles']:
        remaining={t['id']:dependencies(t,profile) for t in tasks if profile in t['profiles']}
        done=set()
        while remaining:
            ready={k for k,ds in remaining.items() if ds<=done}
            assert ready,f'{profile}: cycle or missing profile dependency: {remaining}'
            done |= ready
            remaining={k:ds for k,ds in remaining.items() if k not in ready}
    return index

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--render',action='store_true',help='regenerate task contracts only')
    parser.add_argument('--profile',choices=['pilot','paid'],default='paid')
    parser.add_argument('--completed',default='',help='comma-separated accepted IDs for readiness preview only')
    parser.add_argument('--waves',action='store_true',help='print topological waves; excludes authority/capacity gates')
    args=parser.parse_args(); plan=json.loads(PLAN.read_text()); index=validate(plan)
    for task in plan['tasks']:
        path=CONTRACTS/f"{task['id']}.md"; expected=render(task)
        if args.render:
            path.parent.mkdir(parents=True,exist_ok=True);path.write_text(expected)
        else:
            assert path.exists() and path.read_text()==expected, f'{path}: missing/stale; run --render'
    print(f"PASS: {len(index)} contracts; both profile DAGs acyclic; original40 requirements covered; generated contracts match.")
    if args.waves:
        pending={k:dependencies(t,args.profile) for k,t in index.items() if args.profile in t['profiles']};done=set();wave=0
        while pending:
            ready=sorted((k for k,ds in pending.items() if ds<=done),key=lambda x:int(x.split('.')[1]))
            print(f"Wave{wave}: {', '.join(ready)}")
            done.update(ready);pending={k:ds for k,ds in pending.items() if k not in ready};wave+=1
    completed={x for x in args.completed.split(',') if x};assert completed<=set(index),'unknown completed ID'
    for tid in completed:
        assert args.profile in index[tid]['profiles'],'completed task not in profile'
        assert dependencies(index[tid],args.profile)<=completed,f'{tid}: incomplete dependency closure'
    ready=[t for t in plan['tasks'] if args.profile in t['profiles'] and t['id'] not in completed and dependencies(t,args.profile)<=completed]
    batch=[];held=[]
    for task in ready:
        conflicting=[other['id'] for other in batch if (any(overlap(a,b) for a in task['write_paths'] for b in other['write_paths']) or set(task['resource_claims']+task['live_resource_claims']) & set(other['resource_claims']+other['live_resource_claims']))]
        if conflicting:held.append((task['id'],conflicting))
        else:batch.append(task)
    print('Dependency/file/resource-safe candidate batch: '+', '.join(t['id'] for t in batch))
    for tid,conflicts in held:print(f'Hold concurrent {tid}: shares files/resources with {conflicts}')
    for t in batch:
        if t['external_gates']:print(f"{t['id']} live gates still required: {', '.join(t['external_gates'])}")
    print('Preview only: no claims, workers, approvals, spending, or completion evidence were created/verified.')

if __name__=='__main__':main()
