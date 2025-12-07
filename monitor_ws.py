import asyncio
import websockets
import json
import sys
import logging

async def monitor():
    uri = "ws://localhost:8081/ws"
    print(f"Connecting to {uri}...")
    try:
        async with websockets.connect(uri) as websocket:
            print("Connected. Waiting for messages...")
            count = 0
            while count < 500:
                message = await websocket.recv()
                data = json.loads(message)
                if 'pressure' in data:
                    print(f"FOUND BARO DATA: {data}")
                    count += 1
                elif count == 0 and ('x' in data):
                     # Print first few pos messages to verify
                     print(f"POS: {data}")
            print(f"Captured {count} messages with baro data (or position data if no baro yet).")
    except websockets.exceptions.InvalidMessage as e:
        print(f"WebSocket Error: {e}")
        # InvalidMessage does not have headers, status_code etc directly
    except Exception as e:
        print(f"General Error: {type(e).__name__}: {e}")

if __name__ == "__main__":
    try:
        asyncio.run(monitor())
    except KeyboardInterrupt:
        pass
