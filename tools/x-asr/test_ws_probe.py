#!/usr/bin/env python3
"""Quick X-ASR WebSocket probe: connect, send tone, read final."""
import asyncio
import json
import math
import struct

import websockets


async def main():
    uri = "ws://127.0.0.1:8766"
    async with websockets.connect(uri) as ws:
        await ws.send(json.dumps({"type": "start", "sample_rate": 16000}))
        msg = json.loads(await ws.recv())
        print("start:", msg)

        sr = 16000
        samples = [int(8000 * math.sin(2 * math.pi * 440 * i / sr)) for i in range(sr)]
        pcm = struct.pack("<" + "h" * len(samples), *samples)
        for i in range(0, len(pcm), 640):
            await ws.send(pcm[i : i + 640])

        await ws.send(json.dumps({"type": "end"}))
        while True:
            r = await asyncio.wait_for(ws.recv(), 10)
            print("recv:", r[:300] if isinstance(r, str) else "binary")
            if isinstance(r, str):
                m = json.loads(r)
                if m.get("type") in ("final", "error"):
                    break


if __name__ == "__main__":
    asyncio.run(main())
