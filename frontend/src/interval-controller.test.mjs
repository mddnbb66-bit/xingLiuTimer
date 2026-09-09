import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
const source = await readFile(new URL('./interval-controller.js', import.meta.url), 'utf8');
const { createIntervalController } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);

test('快速编辑只保存最终值，空值和越界不发送', async () => {
 const saves=[];const applied=[];
 const controller=createIntervalController({save:async v=>{saves.push(v);return {};},applied:v=>applied.push(v),status:()=>{}});
 assert.equal(controller.schedule(NaN),false);
 assert.equal(controller.schedule(0),false);
 assert.equal(controller.schedule(241),false);
 controller.schedule(4);controller.schedule(45);await controller.flush();
 assert.deepEqual(saves,[45]);assert.deepEqual(applied,[45]);
});
test('旧请求完成后不会覆盖最新间隔',async()=>{
 let release;const saves=[];const applied=[];
 const controller=createIntervalController({save:v=>{saves.push(v);return v===15?new Promise(r=>{release=r;}):Promise.resolve({});},applied:v=>applied.push(v),status:()=>{}});
 controller.schedule(15,true);controller.schedule(60);
 const done=controller.flush();release({});await done;
 assert.deepEqual(saves,[15,60]);assert.deepEqual(applied,[60]);
});
test('失败会报告错误并允许重试',async()=>{
 let failing=true;let applied=false;const messages=[];
 const controller=createIntervalController({save:async()=>{if(failing)throw new Error('disk full');return {};},applied:()=>{applied=true;},status:m=>messages.push(m)});
 controller.schedule(20);await assert.rejects(controller.flush(),/disk full/);
 assert.equal(applied,false);assert.match(messages.at(-1),/保存失败/);
 failing=false;controller.schedule(20);await controller.flush();assert.equal(applied,true);
});
