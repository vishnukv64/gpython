import asyncio

async def handle(reader, writer):
    while data := await reader.readline():
        writer.write(b"echo:" + data)
        await writer.drain()
    writer.close()
    await writer.wait_closed()

async def main():
    server = await asyncio.start_server(handle, "127.0.0.1", 0)
    port = server.sockets[0].getsockname()[1]
    async with server:
        reader, writer = await asyncio.open_connection("127.0.0.1", port)
        for msg in (b"hello\n", b"world\n"):
            writer.write(msg)
            await writer.drain()
            print(await reader.readline())
        writer.close()
        await writer.wait_closed()
    print("closed")

asyncio.run(main())
