
class ORAMtree:
    def __init__(self, L, Z):
        self.L = L
        self.tree = {}

        for h in range(self.L + 1):

            for i in range(2**h):

                if h == 0:
                    key = ""
                else:
                    key = format(i, f"0{h}b")

                b = bucket(Z)
                b.generate_dummy()

                self.tree[key] = b

    def __repr__(self):

        out = []

        for h in range(self.L + 1):

            out.append(f"Level {h}")

            for i in range(2**h):

                if h == 0:
                    key = ""
                else:
                    key = format(i, f"0{h}b")

                out.append(f"  [{key}] {self.tree[key]}")

        return "\n".join(out)
    
    def read_bucket(self, leaf, h):
        key = leaf[:h]
        return self.tree[key]
    
    def read_path(self,leaf):
        datablocks = list()
        for i in range(self.L+1):
            for j in self.read_bucket(leaf,i):
                datablocks.append(j)
        return datablocks
    
    def set_bukcket(self, datadict):
        for k,v in datadict:
            self.tree[k] = v


class bucket:
    def __init__(self,Z):
        self.Z = Z
        self.value = list()
    
    def generate_dummy(self):
        for _ in range(self.Z):
            self.value.append(datablock(0,0,""))
            
    def __repr__(self):
        return "[" + ",".join(repr(i) for i in self.value) + "]"


class datablock:
    def __init__(self,addr,path,data):
        self.addr = address(addr)
        self.path = path
        self.data = data
    
    def __repr__(self):
        if self.addr.isdummy():
            return "dummy block"
        return f"addr:{self.addr} path:{self.path} data:{self.data}"


class address:
    def __init__(self,address):
        if address == 0:
            self.addr = None
        else:
            self.addr = address

    def isdummy(self):
        return self.addr is None
        
    def __repr__(self):
        return f"address = {self.addr}"
    

pm = dict()
stash = list()

#128のデータを考える
N = 128
L = 6
Z = 4


tree = ORAMtree(L, Z)



