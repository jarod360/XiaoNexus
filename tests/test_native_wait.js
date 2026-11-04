'use strict';
const fs=require('fs'),vm=require('vm'),assert=require('assert');
const text=fs.readFileSync(require('path').join(__dirname,'..','runtime','frida','lx04_nexus_gate.js'),'utf8');
const part=text.split('// BEGIN_NATIVE_WAIT_CONTROLLER')[1].split('// END_NATIVE_WAIT_CONTROLLER')[0];
function setup(){let now=0,id=0,closed=[],disposed=[],queue=[],timers=new Map(),notes=[];
 const v={name:'native'};let available=true;
 const box={};vm.createContext(box);vm.runInContext(part,box);
 const c=box.makeNativeWait({now:()=>now,main:f=>queue.push(f),timer:(f,ms)=>{timers.set(++id,{f,at:now+ms});return id;},clear:i=>timers.delete(i),acquire:()=>available?v:null,start:()=>{},dismiss:w=>closed.push(w),dispose:w=>disposed.push(w),same:(a,b)=>a===b,note:n=>notes.push(n),sample:()=>{}});
 return {c,v,closed,notes,flush:()=>{while(queue.length)queue.shift()();},advance:ms=>{now+=ms;for(const[i,t]of timers)if(t.at<=now){timers.delete(i);t.f();}},unavailable:()=>available=false};}
let n=0;function test(name,f){f();n++;console.log('PASS',name)}
test('request end releases native dialog',()=>{const s=setup();s.c.begin('a');s.flush();assert(s.c.owns(s.v));s.c.end('a',true,'REQUEST_END');s.flush();assert.equal(s.closed.length,1)});
test('stale request cannot cancel newer wait',()=>{const s=setup();s.c.begin('b');s.flush();s.c.end('a',true,'OLD');s.flush();assert(s.c.owns(s.v))});
test('new wakeup cancels queued begin',()=>{const s=setup();s.c.begin('a');s.c.end(null,false,'WAKE');s.flush();assert(!s.c.owns(s.v))});
test('request end cancels queued begin',()=>{const s=setup();s.c.begin('a');s.c.end('a',true,'END');s.flush();assert(!s.c.owns(s.v))});
test('new wakeup invalidates queued dismiss',()=>{const s=setup();s.c.begin('a');s.flush();s.c.end('a',true,'END');s.c.end(null,false,'WAKE');s.flush();assert.equal(s.closed.length,0)});
test('doorbell exits active wait',()=>{const s=setup();s.c.begin('a');s.flush();s.c.end(null,true,'BLOCK');s.flush();assert.equal(s.closed.length,1)});
test('47 second upper bound releases wait',()=>{const s=setup();s.c.begin('a');s.flush();s.advance(47000);s.flush();assert(!s.c.owns(s.v));assert.equal(s.closed.length,1)});
test('long wait remains owned before upper bound',()=>{const s=setup();s.c.begin('a');s.flush();s.advance(30000);assert(s.c.owns(s.v));assert.equal(s.c.remaining(),17000)});
test('missing existing card never creates UI',()=>{const s=setup();s.unavailable();s.c.begin('a');s.flush();assert(!s.c.owns(s.v));assert.equal(s.closed.length,0)});
test('repeated cancellation preserves queued cleanup',()=>{const s=setup();s.c.begin('a');s.flush();s.c.end('a',true,'END');s.c.end(null,true,'BLOCK');s.flush();assert.equal(s.closed.length,1)});
console.log(n+' native wait lifecycle tests PASS');
