const WORKER_CODE = `
'use strict';

function sha256(data) {
  var h0=0x6a09e667,h1=0xbb67ae85,h2=0x3c6ef372,h3=0xa54ff53a;
  var h4=0x510e527f,h5=0x9b05688c,h6=0x1f83d9ab,h7=0x5be0cd19;
  var k=[0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,
    0x923f82a4,0xab1c5ed5,0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,
    0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,0xe49b69c1,0xefbe4786,
    0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
    0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,
    0x06ca6351,0x14292967,0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,
    0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,0xa2bfe8a1,0xa81a664b,
    0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
    0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,
    0x5b9cca4f,0x682e6ff3,0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,
    0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2];
  var bytes=[];
  for(var i=0;i<data.length;i++){
    bytes.push(data.charCodeAt(i));
  }
  bytes.push(0x80);
  while((bytes.length%64)!==56) bytes.push(0);
  var bitLen=data.length*8;
  for(var i=7;i>=0;i--) bytes.push((bitLen>>>(i*8))&0xff);
  for(var chunk=0;chunk<bytes.length;chunk+=64){
    var w=[];
    for(var i=0;i<16;i++){
      w[i]=(bytes[chunk+i*4]<<24)|(bytes[chunk+i*4+1]<<16)|
            (bytes[chunk+i*4+2]<<8)|bytes[chunk+i*4+3];
    }
    for(var i=16;i<64;i++){
      var s0=rr(w[i-15],7)^rr(w[i-15],18)^(w[i-15]>>>3);
      var s1=rr(w[i-2],17)^rr(w[i-2],19)^(w[i-2]>>>10);
      w[i]=(w[i-16]+s0+w[i-7]+s1)|0;
    }
    var a=h0,b=h1,c=h2,d=h3,e=h4,f=h5,g=h6,hh=h7;
    for(var i=0;i<64;i++){
      var S1=rr(e,6)^rr(e,11)^rr(e,25);
      var ch=(e&f)^((~e)&g);
      var t1=(hh+S1+ch+k[i]+w[i])|0;
      var S0=rr(a,2)^rr(a,13)^rr(a,22);
      var maj=(a&b)^(a&c)^(b&c);
      var t2=(S0+maj)|0;
      hh=g;g=f;f=e;e=(d+t1)|0;d=c;c=b;b=a;a=(t1+t2)|0;
    }
    h0=(h0+a)|0;h1=(h1+b)|0;h2=(h2+c)|0;h3=(h3+d)|0;
    h4=(h4+e)|0;h5=(h5+f)|0;h6=(h6+g)|0;h7=(h7+hh)|0;
  }
  function rr(n,b){return(n>>>b)|(n<<(32-b));}
  function hex(n){var s='';for(var i=7;i>=0;i--)s+=((n>>>(i*4))&0xf).toString(16);return s;}
  return hex(h0)+hex(h1)+hex(h2)+hex(h3)+hex(h4)+hex(h5)+hex(h6)+hex(h7);
}

function hasLeadingZeroBits(hashHex, bits) {
  var fullNibbles = Math.floor(bits / 4);
  var remainBits = bits % 4;
  for (var i = 0; i < fullNibbles; i++) {
    if (hashHex[i] !== '0') return false;
  }
  if (remainBits > 0) {
    var nibble = parseInt(hashHex[fullNibbles], 16);
    var mask = (0xF << (4 - remainBits)) & 0xF;
    if ((nibble & mask) !== 0) return false;
  }
  return true;
}

self.onmessage = function(e) {
  var id = e.data.id;
  var nonce = e.data.nonce;
  var difficulty = e.data.difficulty;
  var batchSize = 10000;

  for (var i = 0; i < 100000000; i++) {
    var solution = i.toString(16);
    var input = id + ':' + nonce + ':' + solution;
    var hash = sha256(input);
    if (hasLeadingZeroBits(hash, difficulty)) {
      self.postMessage({ type: 'solved', solution: solution, iterations: i });
      return;
    }
    if (i % batchSize === 0 && i > 0) {
      self.postMessage({ type: 'progress', iterations: i });
    }
  }
  self.postMessage({ type: 'failed' });
};
`;

export interface PoWResult {
  solution: string;
  iterations: number;
}

export function solvePoW(
  id: string,
  nonce: string,
  difficulty: number,
  onProgress?: (iterations: number) => void
): Promise<PoWResult> {
  return new Promise((resolve, reject) => {
    const blob = new Blob([WORKER_CODE], { type: 'application/javascript' });
    const url = URL.createObjectURL(blob);
    const worker = new Worker(url);

    worker.onmessage = (e) => {
      const msg = e.data;
      if (msg.type === 'solved') {
        worker.terminate();
        URL.revokeObjectURL(url);
        resolve({ solution: msg.solution, iterations: msg.iterations });
      } else if (msg.type === 'progress') {
        onProgress?.(msg.iterations);
      } else if (msg.type === 'failed') {
        worker.terminate();
        URL.revokeObjectURL(url);
        reject(new Error('PoW solving failed: max iterations exceeded'));
      }
    };

    worker.onerror = (err) => {
      worker.terminate();
      URL.revokeObjectURL(url);
      reject(err);
    };

    worker.postMessage({ id, nonce, difficulty });
  });
}
