import random

class Point:
    def __init__(self, num, n):
        if num == "random":
            self.value = int(bin(random.randint(0,2**n-1)),2)
        else:
            self.value = int(num, 2) if isinstance(num, str) else int(num)
        self.n = n

    def __repr__(self):
        return format(self.value, f"0{self.n}b")

    def __xor__(self, other):
        if isinstance(other, int):
            return Point(self.value ^ other, self.n)

        if isinstance(other, Point):
            return Point(self.value ^ other.value, self.n)

        return NotImplemented

    def __eq__(self, other):
        if self.value == other.value and self.n == other.n:
            return True
        else:
            return False

    def nextpath(self):
        result = []

        for i in range(self.n):
            result.append(self ^ (1 << i))

        return result
    
    def random_next_point(self):
        i = random.randint(0,self.n-1)
        next_point_list = self.nextpath()
        return next_point_list[i]
    
    def adjacement(self,target):
        result = self.value ^ target.value
        return (result & (result-1)) == 0
    
    
class hypercube:
    def __init__(self,n):
        self.n = n
    
    def ramdom_path(self,step):
        path = list()

        start = Point('random',self.n)

        path.append(start)
        p = start
        while len(path) != (step+1):
            p = p.random_next_point()
            if p in path:
                continue
            path.append(p)
        
        return path
    
    def random_inducedpath(self,step):
        path = list()

        start = Point('random',self.n)

        path.append(start)
        p = start
        while len(path) != (step+1):
            next_list = p.nextpath()

            noadjacement = list()

            flg = 0

            for a in next_list:
                if a in path:
                    continue

                for b in path[:-1]:
                    if a.adjacement(b):
                        flg = 1
                        break
                if flg == 0:
                    noadjacement.append(a)
                flg = 0

            if len(noadjacement) == 0:
                raise ValueError("No adjacement point missing")

            i = random.randint(0,len(noadjacement)-1)

            path.append(noadjacement[i])

            p = noadjacement[i]
        
        return path
            


def compare_list(a, b):
    for x in a:
        if x in b:
            return True
    return False


n = 8
graph = hypercube(n)

def stepVSoverlap(step):
    trial = 1000
    true_counter = 0

    for i in range(trial):
        a = graph.ramdom_path(step)
        b = graph.ramdom_path(step)

        if compare_list(a,b):
            true_counter += 1 

    print(f"step = {step} Probability = {true_counter/trial} normal_path")

def stepVSoverlap_includepath(step):
    trial = 1000
    true_counter = 0

    for i in range(trial):
        a = graph.random_inducedpath(step)
        b = graph.random_inducedpath(step)

        if compare_list(a,b):
            true_counter += 1 

    print(f"step = {step} Probability = {true_counter/trial} include_path")

for i in range(2,40,2):
    stepVSoverlap(i)
    stepVSoverlap_includepath(i)


