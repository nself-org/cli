"""Redact generated smoke credentials by value before diagnostic output."""
import pathlib
import re
import sys


def redact(text, values):
    for value in sorted(set(values), key=len, reverse=True):
        if value:
            text = text.replace(value, '<secret>')
    text = re.sub(r'eyJ[A-Za-z0-9_.-]+', '<jwt>', text)
    return re.sub(r'[A-Fa-f0-9]{32,}', '<hex>', text)


def secret_values(work, project):
    values = []
    admin = pathlib.Path(work) / 'admin.secret'
    if admin.is_file():
        values.append(admin.read_text().strip())
    if project:
        for env in pathlib.Path(project).glob('.env*'):
            if not env.is_file():
                continue
            for line in env.read_text().splitlines():
                key, sep, value = line.partition('=')
                if sep and re.search(r'PASSWORD|SECRET|TOKEN|KEY', key):
                    values.append(value.strip().strip('\"\''))
    return values


if __name__ == '__main__':
    sys.stdout.write(redact(sys.stdin.read(), secret_values(*sys.argv[1:3])))
