"""
AES-based pseudo-random generator for experiments.

Usage:
    import PRF

    b = PRF.random_bytes(123, nbytes=32)
    x = PRF.random_int(123, bits=64)

This module is intentionally simple and deterministic: the same public_input
and key produce the same output. It is meant for experiments, not production
security.
"""

from __future__ import annotations

import hashlib
from typing import Optional


# Fixed experiment key. AES keys must be 16, 24, or 32 bytes.
# Change this value if you want a different deterministic sequence.
DEFAULT_KEY = b"TERM2026_AES_PRF"

BLOCK_SIZE = 16


def _make_aes_cipher(key: bytes):
    """Create an AES-ECB cipher using an installed AES backend."""
    try:
        from Crypto.Cipher import AES  # type: ignore

        return AES.new(key, AES.MODE_ECB)
    except ImportError:
        pass

    try:
        from cryptography.hazmat.backends import default_backend  # type: ignore
        from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes  # type: ignore

        cipher = Cipher(algorithms.AES(key), modes.ECB(), backend=default_backend())
        encryptor = cipher.encryptor()

        class _CryptographyAES:
            def encrypt(self, block: bytes) -> bytes:
                return encryptor.update(block)

        return _CryptographyAES()
    except ImportError as exc:
        raise ImportError(
            "AES backend is not installed. Install one of these:\n"
            "  pip install pycryptodome\n"
            "  pip install cryptography"
        ) from exc


def _normalize_key(key: Optional[bytes]) -> bytes:
    if key is None:
        key = DEFAULT_KEY
    if not isinstance(key, bytes):
        raise TypeError("key must be bytes")
    if len(key) not in (16, 24, 32):
        raise ValueError("AES key length must be 16, 24, or 32 bytes")
    return key


def _input_prefix(public_input: int) -> bytes:
    if not isinstance(public_input, int):
        raise TypeError("public_input must be int")

    # Hash the public integer into 8 bytes so there is no practical size limit.
    digest = hashlib.sha256(str(public_input).encode("ascii")).digest()
    return digest[:8]


def random_bytes(public_input: int, nbytes: int = 32, key: Optional[bytes] = None) -> bytes:
    """
    Return deterministic pseudo-random bytes from a public integer input.

    Args:
        public_input: Public integer used as the seed/input.
        nbytes: Number of output bytes.
        key: Optional AES key. If omitted, DEFAULT_KEY is used.
    """
    if nbytes < 0:
        raise ValueError("nbytes must be non-negative")

    aes_key = _normalize_key(key)
    cipher = _make_aes_cipher(aes_key)
    prefix = _input_prefix(public_input)

    out = bytearray()
    counter = 0
    while len(out) < nbytes:
        block = prefix + counter.to_bytes(8, "big")
        out.extend(cipher.encrypt(block))
        counter += 1

    return bytes(out[:nbytes])


def random_int(public_input: int, bits: int = 128, key: Optional[bytes] = None) -> int:
    """Return a deterministic pseudo-random non-negative integer."""
    if bits <= 0:
        raise ValueError("bits must be positive")

    nbytes = (bits + 7) // 8
    value = int.from_bytes(random_bytes(public_input, nbytes, key), "big")
    extra_bits = nbytes * 8 - bits
    if extra_bits:
        value >>= extra_bits
    return value


def random_float(public_input: int, key: Optional[bytes] = None) -> float:
    """Return a deterministic pseudo-random float in [0.0, 1.0)."""
    value = random_int(public_input, bits=53, key=key)
    return value / (1 << 53)


def prg(public_input: int, nbytes: int = 32, key: Optional[bytes] = None) -> bytes:
    """Alias for random_bytes."""
    return random_bytes(public_input, nbytes=nbytes, key=key)


def prf(public_input: int, bits: int = 128, key: Optional[bytes] = None) -> int:
    """Alias for random_int."""
    return random_int(public_input, bits=bits, key=key)


if __name__ == "__main__":
    seed = 123
    print(random_bytes(seed, 16).hex())
    print(random_int(seed, 64))
    print(random_float(seed))
