import unittest
import pathlib
import tempfile
from redact import redact, secret_values


class RedactTest(unittest.TestCase):
    def test_values(self):
        for text, values, want in [
            ('error: mixed+/value=', ['mixed+/value='], 'error: <secret>'),
            ('long-token and long', ['long', 'long-token'], '<secret> and <secret>'),
            ('safe', [''], 'safe'),
            ('a' * 40, [], '<hex>'),
        ]:
            with self.subTest(text=text):
                self.assertEqual(redact(text, values), want)

    def test_private_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            (root / 'admin.secret').write_text('fake-admin+/value=')
            (root / '.env').write_text('POSTGRES_PASSWORD="fake-password+/value="\nPROJECT_NAME=public\n')
            (root / '.env.secrets').write_text("HASURA_JWT_KEY='fake-jwt+/value='\n")
            values = secret_values(directory, directory)
            for secret in ('fake-admin+/value=', 'fake-password+/value=', 'fake-jwt+/value='):
                with self.subTest(secret=secret):
                    self.assertEqual(redact(secret, values), '<secret>')
            self.assertEqual(redact('public', values), 'public')


if __name__ == '__main__':
    unittest.main()
