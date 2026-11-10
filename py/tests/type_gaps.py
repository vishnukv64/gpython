def t(f):
    try: print(repr(f()))
    except BaseException as e: print(type(e).__name__, e)
for s in ['ß','ﬁ','İ','ǆ','ΑΣ','ŉ','ǰ']:
    for m in ['upper','lower','title','capitalize','swapcase','casefold']:
        t(lambda: getattr(s,m)())
inf=float('inf');nan=float('nan')
t(lambda:round(inf));t(lambda:round(nan));t(lambda:round(inf,2));t(lambda:round(nan,2))
t(lambda:1/0.0);t(lambda:5.0//0);t(lambda:5.0%0);t(lambda:divmod(5.0,0));t(lambda:5%0.0);t(lambda:divmod(5,0.0));t(lambda:5//0.0);t(lambda:1.0/0)
try: 10.0**400
except OverflowError as e: print(type(e).__name__, e.args[0], len(e.args))
try: 10**400.0
except OverflowError as e: print(type(e).__name__, e.args[0], len(e.args))
t(lambda:(-8)**(1/3)==(-8.0)**(1/3))
t(lambda:type((-8)**(1/3)))
t(lambda:[round(x,9) for x in ((-8)**(1/3)).real.__class__(1).as_integer_ratio()])
t(lambda:round(((-8)**(1/3)).imag,9))
t(lambda:round(((-8)**(1/3)).real,9))
t(lambda:0.0**-1);t(lambda:2**0.5)
t(lambda:{1.0:'a',True:'b'});t(lambda:{1:'x',1.0:'y',True:'z'});t(lambda:{1,1.0,True});t(lambda:{0,0.0,False,-0.0});t(lambda:{2**70:1,float(2**70):2})
t(lambda:{1:1}.get(1.0));t(lambda:{(1,2):1}[(1.0,2)]);t(lambda:frozenset([1,1.0]));t(lambda:{nan:1}.get(nan))
t(lambda:{[1]:2});t(lambda:{}[[1]]);t(lambda:{[1]});t(lambda:frozenset([[1]]));t(lambda:set([[1]]));t(lambda:{}.get([1]));t(lambda:{{}:1});t(lambda:{1:2}.pop([1]));t(lambda:set().add([1]));t(lambda:[1] in {1});t(lambda:{1:1}.setdefault([1]))
t(lambda:{(1,[2]):1})
import json
t(lambda:json.dumps('é€😀',ensure_ascii=False));t(lambda:json.dumps('é€😀'));t(lambda:json.dumps({'é':'ü'},ensure_ascii=False,indent=1))
t(lambda:json.dumps(object()));t(lambda:json.dumps({1:set()}));t(lambda:json.dumps(float('nan')));t(lambda:json.dumps(float('nan'),allow_nan=False))
for s in ['{','','[1,]','{"a":1,}','tru','{"a" 1}','[1,2\n,x]','"abc','"\\q"','1 2','{1:2}','nul','"a\nb"']:
    t(lambda:json.loads(s))
try: json.loads('[1,\n 2,\n x]')
except json.JSONDecodeError as e: print(e.msg,e.lineno,e.colno,e.pos,e.doc)
t(lambda:json.loads(b'[1]'));t(lambda:json.loads(None))
e=OSError(2,'msg','f');print(e.errno,e.strerror,e.filename,e.filename2,e.args,str(e),repr(e),type(e))
for a in [(2,'x'),(13,'x'),(17,'x'),(1,'x'),(),(5,),('a',),(2,'x','f',None,'g'),(32,'x')]:
    e=OSError(*a);print(type(e).__name__,e.errno,e.strerror,e.filename,e.filename2,e.args,str(e),repr(e))
# errno numbers differ by OS (EAGAIN is 11 on Linux, 35 on macOS), so map by name
import errno
for n in ['EAGAIN','ETIMEDOUT','ECHILD','EINTR','ECONNREFUSED','ESRCH','EISDIR','ENOTDIR']:
    e=OSError(getattr(errno,n),'x');print(n,type(e).__name__,e.errno==getattr(errno,n),str(e)=='[Errno %d] x'%getattr(errno,n))
e=FileNotFoundError(2,'x','f');print(type(e).__name__,e.errno,str(e))
e=FileNotFoundError('x');print(e.errno,e.args,str(e))
class M(OSError):pass
e=M(2,'x');print(type(e).__name__,e.errno,e)
t(lambda:"{:>{w}}".format("x",w=5));t(lambda:"{:{}.{}f}".format(3.14159,10,2));t(lambda:"{0:{1}}".format('a',4));t(lambda:"{:{a}{b}}".format('a',a='^',b=5));t(lambda:"{:{:{}}}".format(1,2,3));t(lambda:"{:{w}}".format(1));t(lambda:"{:{0}}".format(1,5))
t(lambda:'{:>{w}}'.format('x',w='z'));t(lambda:'{x[0]:>{y.real}}'.format(x='ab',y=4))
t(lambda:f"{'x':>{5}}");t(lambda:format('x','>{}'))
