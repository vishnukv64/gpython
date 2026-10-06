import mmap

# The one call pip's vendored cachecontrol makes (filewrapper.py):
#   result = memoryview(mmap.mmap(self.__buf.fileno(), 0, access=mmap.ACCESS_READ))
path = "/tmp/gpython_mmap_pipline.bin"
content = b"wheel-bytes-\xc3\xa9\x00\xff-packaged"
fp = open(path, "wb")
fp.write(content)
fp.close()
fp = open(path, "rb")

result = memoryview(mmap.mmap(fp.fileno(), 0, access=mmap.ACCESS_READ))
print("len_match", len(result) == len(content))
print("bytes_match", bytes(result) == content)
fp.close()
print("done")
