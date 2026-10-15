#!/usr/bin/env python3
"""Regenerate apps/cpin-operator/base/programs/*-program.yaml from
programs/<svc>/Pulumi.yaml. --check fails (exit 1) if committed CRs are stale.
"""
import sys, glob, os, io, yaml

BASE = 'apps/cpin-operator/base/programs'
HEADER = '# Generated from programs/{svc}/Pulumi.yaml — edit there, run scripts/gen-programs.py\n'


def render(svc, path):
    prog = yaml.safe_load(open(path))
    cr = {'apiVersion': 'pulumi.com/v1', 'kind': 'Program',
          'metadata': {'name': f'cpin-program-{svc}'}, 'program': prog}
    buf = io.StringIO()
    buf.write(HEADER.format(svc=svc))
    yaml.safe_dump(cr, buf, sort_keys=False)
    return buf.getvalue()


def main():
    check = '--check' in sys.argv
    stale = []
    for path in sorted(glob.glob('programs/*/Pulumi.yaml')):
        svc = os.path.basename(os.path.dirname(path))
        want = render(svc, path)
        out = f'{BASE}/{svc}-program.yaml'
        got = open(out).read() if os.path.exists(out) else ''
        if got != want:
            stale.append(out)
            if not check:
                open(out, 'w').write(want)
                print(f'regenerated {out}')
    if check and stale:
        print('STALE:', ', '.join(stale)); sys.exit(1)
    print('OK' if not stale else 'done')


if __name__ == '__main__':
    main()
