import{u as e}from"./index-r5yGN_LK.js";var t=class extends Error{code;format;details;cause;constructor(e,t,n={}){super(t),this.name=`SubtitleDiagnosticError`,this.code=e,this.format=n.format,this.details=n.details,this.cause=n.cause}};function n(e,n={}){if(e instanceof t)return e;let r=e instanceof Error?e:Error(String(e));return new t(o(r.message,n.fallbackCode),r.message,{format:n.format,details:n.details,cause:e})}function r(e,t,n={}){return{code:e,message:t,format:n.format,cueIndex:n.cueIndex,details:n.details}}function i(e,t={}){let n=e?.trim().toUpperCase();if(!n)return null;switch(n){case`MISSING_PALETTE`:return r(`MISSING_PALETTE`,`PGS cue references a palette that was not available at render time.`,{format:t.format,cueIndex:t.cueIndex});case`INVALID_PACKET`:return r(`INVALID_SUBTITLE_DATA`,`Subtitle packet could not be decoded for the requested cue.`,{format:t.format,cueIndex:t.cueIndex});case`RENDER_CONTEXT_UNAVAILABLE`:return r(`INVALID_SUBTITLE_DATA`,`Subtitle render context could not be assembled for the requested cue.`,{format:t.format,cueIndex:t.cueIndex});case`EMPTY_RENDER`:return r(`INVALID_SUBTITLE_DATA`,`Subtitle cue rendered without any visible bitmap data.`,{format:t.format,cueIndex:t.cueIndex});default:return null}}function a(e){return`[libbitsub:${e.code}] ${e.message}`}function o(e,t=`UNKNOWN`){let n=e.toLowerCase();return n.includes(`detect subtitle format`)||n.includes(`unsupported format`)?`UNSUPPORTED_FORMAT`:n.includes(`no s_vobsub track`)||n.includes(`track not found`)?`TRACK_NOT_FOUND`:n.includes(`idx`)||n.includes(`codecprivate`)||n.includes(`filepos`)?`BAD_IDX`:n.includes(`palette`)?`MISSING_PALETTE`:n.includes(`failed to fetch`)?`FETCH_FAILED`:n.includes(`no `)&&n.includes(`provided`)?`MISSING_INPUT`:n.includes(`invalid`)||n.includes(`truncated`)||n.includes(`malformed`)||n.includes(`subtitle block`)||n.includes(`payload`)?`INVALID_SUBTITLE_DATA`:t}var s=null,c=null;async function l(){if(!s)return c||(c=(async()=>{let t=await e(()=>import(`./libbitsub-BNyL9oBp.js`),[]);await t.default(),s=t})(),c)}function u(){if(!s)throw Error(`WASM module not initialized. Call initWasm() first.`);return s}function d(){try{return new URL(`/assets/libbitsub_bg-BKa5TTeY.wasm`,``+import.meta.url).href}catch{return typeof window<`u`?new URL(`/libbitsub/libbitsub_bg.wasm`,window.location.origin).href:`/libbitsub/libbitsub_bg.wasm`}}function f(){try{return new URL(`/assets/libbitsub-8QOGH3RR.js`,``+import.meta.url).href}catch{return typeof window<`u`?new URL(`/libbitsub/libbitsub.js`,window.location.origin).href:`/libbitsub/libbitsub.js`}}typeof window<`u`&&setTimeout(()=>{l().catch(e=>console.warn(`[libbitsub] WASM pre-init failed:`,e))},100);var p=null,m=null,h=0,g=new Map;function _(){return typeof Worker<`u`&&typeof window<`u`&&typeof Blob<`u`}function v(){return`
let wasmModule = null;
const pgsParsers = new Map();
const vobSubParsers = new Map();

function buildPgsMetadata(parser) {
    return {
        format: 'pgs',
        cueCount: parser.count,
        screenWidth: parser.screenWidth || 0,
        screenHeight: parser.screenHeight || 0
    };
}

function buildVobSubMetadata(parser) {
    return {
        format: 'vobsub',
        cueCount: parser.count,
        screenWidth: parser.screenWidth || 0,
        screenHeight: parser.screenHeight || 0,
        language: parser.language || '',
        trackId: parser.trackId || '',
        hasIdxMetadata: !!parser.hasIdxMetadata
    };
}

function disposeSession(sessionId) {
    const pgsParser = pgsParsers.get(sessionId);
    if (pgsParser) {
        pgsParser.free();
        pgsParsers.delete(sessionId);
    }
    const vobSubParser = vobSubParsers.get(sessionId);
    if (vobSubParser) {
        vobSubParser.free();
        vobSubParsers.delete(sessionId);
    }
}

async function initWasm(wasmUrl, glueUrl) {
    if (wasmModule) return;

    let jsGlueUrl = glueUrl;
    if (!jsGlueUrl) {
        const derivedUrl = new URL(wasmUrl);
        derivedUrl.pathname = derivedUrl.pathname.replace(/_bg.wasm$/, '.js');
        jsGlueUrl = derivedUrl.href;
    }
    const mod = await import(jsGlueUrl);
    await mod.default({ module_or_path: wasmUrl });
    wasmModule = mod;
}

function convertFrame(frame, isVobSub) {
    const compositions = [];
    if (isVobSub) {
        const rgba = frame.getRgba();
        if (frame.width > 0 && frame.height > 0 && rgba.length === frame.width * frame.height * 4) {
            compositions.push({ rgba, x: frame.x, y: frame.y, width: frame.width, height: frame.height });
        }
        return { width: frame.screenWidth, height: frame.screenHeight, compositions };
    }

    for (let i = 0; i < frame.compositionCount; i++) {
        const comp = frame.getComposition(i);
        if (!comp) continue;
        const rgba = comp.getRgba();
        if (comp.width > 0 && comp.height > 0 && rgba.length === comp.width * comp.height * 4) {
            compositions.push({ rgba, x: comp.x, y: comp.y, width: comp.width, height: comp.height });
        }
    }

    return { width: frame.width, height: frame.height, compositions };
}

function postResponse(response, transfer, id) {
    if (id !== undefined) response._id = id;
    self.postMessage(response, transfer && transfer.length > 0 ? transfer : undefined);
}

self.onmessage = async function(event) {
    const { _id, ...request } = event.data;

    try {
        switch (request.type) {
            case 'init': {
                await initWasm(request.wasmUrl, request.glueUrl);
                postResponse({ type: 'initComplete', success: true }, [], _id);
                break;
            }
            case 'loadPgs': {
                disposeSession(request.sessionId);
                const parser = new wasmModule.PgsParser();
                const count = parser.parse(new Uint8Array(request.data));
                const timestamps = parser.getTimestamps();
                pgsParsers.set(request.sessionId, parser);
                postResponse(
                    { type: 'pgsLoaded', count, byteLength: request.data.byteLength, metadata: buildPgsMetadata(parser), timestamps },
                    [timestamps.buffer],
                    _id
                );
                break;
            }
            case 'beginPgs': {
                disposeSession(request.sessionId);
                const parser = new wasmModule.PgsParser();
                parser.reset();
                pgsParsers.set(request.sessionId, parser);
                const timestamps = parser.getTimestamps();
                postResponse(
                    { type: 'pgsProgress', count: 0, added: 0, partial: true, metadata: buildPgsMetadata(parser), timestamps },
                    [timestamps.buffer],
                    _id
                );
                break;
            }
            case 'appendPgs': {
                let parser = pgsParsers.get(request.sessionId);
                if (!parser) {
                    parser = new wasmModule.PgsParser();
                    parser.reset();
                    pgsParsers.set(request.sessionId, parser);
                }
                const added = parser.feed(new Uint8Array(request.data));
                const timestamps = parser.getTimestamps();
                postResponse(
                    { type: 'pgsProgress', count: parser.count, added, partial: true, metadata: buildPgsMetadata(parser), timestamps },
                    [timestamps.buffer],
                    _id
                );
                break;
            }
            case 'finishPgs': {
                const parser = pgsParsers.get(request.sessionId);
                if (!parser) {
                    postResponse({ type: 'error', message: 'PGS session not found for finishPgs' }, [], _id);
                    break;
                }
                const count = parser.finishFeed();
                const timestamps = parser.getTimestamps();
                postResponse(
                    { type: 'pgsProgress', count, added: 0, partial: false, metadata: buildPgsMetadata(parser), timestamps },
                    [timestamps.buffer],
                    _id
                );
                break;
            }
            case 'loadVobSub': {
                disposeSession(request.sessionId);
                const parser = new wasmModule.VobSubParser();
                parser.loadFromData(request.idxContent, new Uint8Array(request.subData));
                const timestamps = parser.getTimestamps();
                vobSubParsers.set(request.sessionId, parser);
                postResponse(
                    { type: 'vobSubLoaded', count: parser.count, metadata: buildVobSubMetadata(parser), timestamps },
                    [timestamps.buffer],
                    _id
                );
                break;
            }
            case 'loadVobSubIdx': {
                disposeSession(request.sessionId);
                const parser = new wasmModule.VobSubParser();
                parser.loadFromIdx(request.idxContent);
                const timestamps = parser.getTimestamps();
                vobSubParsers.set(request.sessionId, parser);
                postResponse(
                    {
                        type: 'vobSubProgress',
                        count: parser.count,
                        partial: true,
                        hasSubData: !!parser.hasSubData,
                        metadata: buildVobSubMetadata(parser),
                        timestamps
                    },
                    [timestamps.buffer],
                    _id
                );
                break;
            }
            case 'attachVobSubData': {
                const parser = vobSubParsers.get(request.sessionId);
                if (!parser) {
                    postResponse({ type: 'error', message: 'VobSub session not found for attachVobSubData' }, [], _id);
                    break;
                }
                parser.attachSubData(new Uint8Array(request.subData));
                const timestamps = parser.getTimestamps();
                postResponse(
                    {
                        type: 'vobSubProgress',
                        count: parser.count,
                        partial: false,
                        hasSubData: !!parser.hasSubData,
                        metadata: buildVobSubMetadata(parser),
                        timestamps
                    },
                    [timestamps.buffer],
                    _id
                );
                break;
            }
            case 'loadVobSubMks': {
                disposeSession(request.sessionId);
                const parser = new wasmModule.VobSubParser();
                parser.loadFromMks(new Uint8Array(request.subData));
                const timestamps = parser.getTimestamps();
                vobSubParsers.set(request.sessionId, parser);
                postResponse(
                    { type: 'vobSubLoaded', count: parser.count, metadata: buildVobSubMetadata(parser), timestamps },
                    [timestamps.buffer],
                    _id
                );
                break;
            }
            case 'loadVobSubOnly': {
                disposeSession(request.sessionId);
                const parser = new wasmModule.VobSubParser();
                parser.loadFromSubOnly(new Uint8Array(request.subData));
                const timestamps = parser.getTimestamps();
                vobSubParsers.set(request.sessionId, parser);
                postResponse(
                    { type: 'vobSubLoaded', count: parser.count, metadata: buildVobSubMetadata(parser), timestamps },
                    [timestamps.buffer],
                    _id
                );
                break;
            }
            case 'renderPgsAtIndex': {
                const parser = pgsParsers.get(request.sessionId);
                if (!parser) { postResponse({ type: 'pgsFrame', frame: null }, [], _id); break; }
                const frame = parser.renderAtIndex(request.index);
                const renderIssue = parser.lastRenderIssue || '';
                if (!frame) { postResponse({ type: 'pgsFrame', frame: null, renderIssue }, [], _id); break; }
                const frameData = convertFrame(frame, false);
                postResponse({ type: 'pgsFrame', frame: frameData, renderIssue }, frameData.compositions.map((c) => c.rgba.buffer), _id);
                break;
            }
            case 'renderVobSubAtIndex': {
                const parser = vobSubParsers.get(request.sessionId);
                if (!parser) { postResponse({ type: 'vobSubFrame', frame: null }, [], _id); break; }
                const frame = parser.renderAtIndex(request.index);
                const renderIssue = parser.lastRenderIssue || '';
                if (!frame) { postResponse({ type: 'vobSubFrame', frame: null, renderIssue }, [], _id); break; }
                const frameData = convertFrame(frame, true);
                postResponse({ type: 'vobSubFrame', frame: frameData, renderIssue }, frameData.compositions.map((c) => c.rgba.buffer), _id);
                break;
            }
            case 'findPgsIndex': {
                const parser = pgsParsers.get(request.sessionId);
                postResponse({ type: 'pgsIndex', index: parser ? parser.findIndexAtTimestamp(request.timeMs) : -1 }, [], _id);
                break;
            }
            case 'findVobSubIndex': {
                const parser = vobSubParsers.get(request.sessionId);
                postResponse({ type: 'vobSubIndex', index: parser ? parser.findIndexAtTimestamp(request.timeMs) : -1 }, [], _id);
                break;
            }
            case 'getPgsTimestamps': {
                const parser = pgsParsers.get(request.sessionId);
                postResponse({ type: 'pgsTimestamps', timestamps: parser ? parser.getTimestamps() : new Float64Array(0) }, [], _id);
                break;
            }
            case 'getVobSubTimestamps': {
                const parser = vobSubParsers.get(request.sessionId);
                postResponse({ type: 'vobSubTimestamps', timestamps: parser ? parser.getTimestamps() : new Float64Array(0) }, [], _id);
                break;
            }
            case 'clearPgsCache': {
                pgsParsers.get(request.sessionId)?.clearCache();
                postResponse({ type: 'cleared' }, [], _id);
                break;
            }
            case 'clearVobSubCache': {
                vobSubParsers.get(request.sessionId)?.clearCache();
                postResponse({ type: 'cleared' }, [], _id);
                break;
            }
            case 'disposePgs': {
                const parser = pgsParsers.get(request.sessionId);
                if (parser) {
                    parser.free();
                    pgsParsers.delete(request.sessionId);
                }
                postResponse({ type: 'disposed' }, [], _id);
                break;
            }
            case 'disposeVobSub': {
                const parser = vobSubParsers.get(request.sessionId);
                if (parser) {
                    parser.free();
                    vobSubParsers.delete(request.sessionId);
                }
                postResponse({ type: 'disposed' }, [], _id);
                break;
            }
            case 'setVobSubDebandEnabled': {
                vobSubParsers.get(request.sessionId)?.setDebandEnabled(request.enabled);
                postResponse({ type: 'debandSet' }, [], _id);
                break;
            }
            case 'setVobSubDebandThreshold': {
                vobSubParsers.get(request.sessionId)?.setDebandThreshold(request.threshold);
                postResponse({ type: 'debandSet' }, [], _id);
                break;
            }
            case 'setVobSubDebandRange': {
                vobSubParsers.get(request.sessionId)?.setDebandRange(request.range);
                postResponse({ type: 'debandSet' }, [], _id);
                break;
            }
        }
    } catch (error) {
        postResponse({ type: 'error', message: error instanceof Error ? error.message : String(error) }, [], _id);
    }
};`}function y(){if(p)return Promise.resolve(p);if(m)return m;let e=b();return m=e,e.then(()=>{m===e&&(m=null)},()=>{m===e&&(m=null)}),e}async function b(){let e=new Blob([v()],{type:`application/javascript`}),t=URL.createObjectURL(e),n;try{n=new Worker(t,{type:`module`})}catch(e){throw URL.revokeObjectURL(t),e instanceof Error?e:Error(String(e))}n.onmessage=e=>{let{_id:t,...n}=e.data;if(t===void 0)return;let r=g.get(t);r&&(g.delete(t),n.type===`error`&&r.requestType===`init`?r.reject(Error(n.message)):r.resolve(n))},n.onerror=e=>{let t=e instanceof ErrorEvent?Error(e.message):Error(String(e));w(n,t),p===n&&(p=null);try{n.terminate()}catch{}};try{let e=await C(n,{type:`init`,wasmUrl:d(),glueUrl:f()});if(e.type===`error`)throw Error(e.message);if(e.type!==`initComplete`||!e.success)throw Error(`Worker WASM initialization failed`);return p=n,n}catch(e){let t=e instanceof Error?e:Error(String(e));w(n,t),p===n&&(p=null);try{n.terminate()}catch{}throw t}finally{URL.revokeObjectURL(t)}}var x=3e4;function S(e,t=x){return p?C(p,e,t):Promise.reject(Error(`Worker not initialized`))}function C(e,t,n=x){return new Promise((r,i)=>{let a=++h,o=setTimeout(()=>{g.delete(a),i(Error(`Worker operation timed out after ${n}ms`))},n);g.set(a,{worker:e,requestType:t.type,resolve:e=>{clearTimeout(o),r(e)},reject:e=>{clearTimeout(o),i(e)}});let s=[];`data`in t&&t.data instanceof ArrayBuffer&&s.push(t.data),`subData`in t&&t.subData instanceof ArrayBuffer&&s.push(t.subData);try{e.postMessage({...t,_id:a},s)}catch(e){g.delete(a),clearTimeout(o),i(e instanceof Error?e:Error(String(e)))}})}function w(e,t){for(let[n,r]of g)r.worker===e&&(g.delete(n),r.reject(t))}var T=`
struct VertexOutput {
  @builtin(position) position: vec4f,
  @location(0) texCoord: vec2f,
}

struct Uniforms {
  resolution: vec2f,
  opacity: f32,
}

struct QuadData {
  destRect: vec4f,   // x, y, w, h in pixels
  texSize: vec4f,    // texW, texH, 0, 0
}

@group(0) @binding(0) var<uniform> uniforms: Uniforms;
@group(0) @binding(1) var<storage, read> quadData: QuadData;

// Quad vertices (two triangles)
const QUAD_POSITIONS = array<vec2f, 6>(
  vec2f(0.0, 0.0),
  vec2f(1.0, 0.0),
  vec2f(0.0, 1.0),
  vec2f(1.0, 0.0),
  vec2f(1.0, 1.0),
  vec2f(0.0, 1.0)
);

@vertex
fn vertexMain(@builtin(vertex_index) vertexIndex: u32) -> VertexOutput {
  var output: VertexOutput;

  let quadPos = QUAD_POSITIONS[vertexIndex];
  let wh = quadData.destRect.zw;

  // Calculate pixel position
  let pixelPos = quadData.destRect.xy + quadPos * wh;

  // Convert to clip space (-1 to 1)
  var clipPos = (pixelPos / uniforms.resolution) * 2.0 - 1.0;
  clipPos.y = -clipPos.y;  // Flip Y for canvas coordinates

  output.position = vec4f(clipPos, 0.0, 1.0);
  output.texCoord = quadPos;

  return output;
}
`,E=`
struct Uniforms {
  resolution: vec2f,
  opacity: f32,
}

@group(0) @binding(0) var<uniform> uniforms: Uniforms;
@group(0) @binding(2) var texSampler: sampler;
@group(0) @binding(3) var tex: texture_2d<f32>;

struct FragmentInput {
  @location(0) texCoord: vec2f,
}

@fragment
fn fragmentMain(input: FragmentInput) -> @location(0) vec4f {
  // Sample pre-multiplied alpha texture (premultiplied on CPU upload)
  return textureSample(tex, texSampler, input.texCoord) * uniforms.opacity;
}
`;function D(){return typeof navigator<`u`&&`gpu`in navigator}var O=class{device=null;context=null;pipeline=null;sampler=null;bindGroupLayout=null;uniformBuffer=null;quadDataBuffers=[];textures=[];pendingDestroyTextures=[];format=`bgra8unorm`;_canvas=null;_initPromise=null;_initialized=!1;_lastCanvasWidth=0;_lastCanvasHeight=0;async init(){return this._initPromise||=this._initDevice(),this._initPromise}async assertShaderModuleValid(e,t){let n=(await e.getCompilationInfo()).messages.filter(e=>e.type===`error`);if(n.length===0)return;let r=n.map(e=>`${t}${e.lineNum>0?`:${e.lineNum}:${e.linePos}`:``} ${e.message}`).join(`
`);throw Error(`WebGPU ${t} shader compilation failed:\n${r}`)}async _initDevice(){if(!navigator.gpu)throw Error(`WebGPU not supported`);let e=await navigator.gpu.requestAdapter({powerPreference:`high-performance`});if(!e)throw Error(`No WebGPU adapter found`);this.device=await e.requestDevice(),this.format=navigator.gpu.getPreferredCanvasFormat();let t=this.device.createShaderModule({code:T}),n=this.device.createShaderModule({code:E});await this.assertShaderModuleValid(t,`vertex`),await this.assertShaderModuleValid(n,`fragment`),this.sampler=this.device.createSampler({magFilter:`linear`,minFilter:`linear`,addressModeU:`clamp-to-edge`,addressModeV:`clamp-to-edge`}),this.uniformBuffer=this.device.createBuffer({size:16,usage:GPUBufferUsage.UNIFORM|GPUBufferUsage.COPY_DST}),this.bindGroupLayout=this.device.createBindGroupLayout({entries:[{binding:0,visibility:GPUShaderStage.VERTEX|GPUShaderStage.FRAGMENT,buffer:{type:`uniform`}},{binding:1,visibility:GPUShaderStage.VERTEX,buffer:{type:`read-only-storage`}},{binding:2,visibility:GPUShaderStage.FRAGMENT,sampler:{type:`filtering`}},{binding:3,visibility:GPUShaderStage.FRAGMENT,texture:{sampleType:`float`}}]});let r=this.device.createPipelineLayout({bindGroupLayouts:[this.bindGroupLayout]});this.pipeline=await this.device.createRenderPipelineAsync({layout:r,vertex:{module:t,entryPoint:`vertexMain`},fragment:{module:n,entryPoint:`fragmentMain`,targets:[{format:this.format,blend:{color:{srcFactor:`one`,dstFactor:`one-minus-src-alpha`,operation:`add`},alpha:{srcFactor:`one`,dstFactor:`one-minus-src-alpha`,operation:`add`}}}]},primitive:{topology:`triangle-list`}}),this._initialized=!0}async setCanvas(e,t,n){if(await this.init(),!this.device)throw Error(`WebGPU device not initialized`);if(!(t<=0||n<=0)){if(this._canvas=e,e.width=t,e.height=n,this._lastCanvasWidth=t,this._lastCanvasHeight=n,!this.context){if(this.context=e.getContext(`webgpu`),!this.context)throw Error(`Could not get WebGPU context`);this.context.configure({device:this.device,format:this.format,alphaMode:`premultiplied`})}this.device.queue.writeBuffer(this.uniformBuffer,0,new Float32Array([t,n,1,0]))}}updateSize(e,t){!this.device||!this._canvas||e<=0||t<=0||(e!==this._lastCanvasWidth||t!==this._lastCanvasHeight)&&(this._canvas.width=e,this._canvas.height=t,this._lastCanvasWidth=e,this._lastCanvasHeight=t,this.device.queue.writeBuffer(this.uniformBuffer,0,new Float32Array([e,t,1,0])))}createTextureInfo(e,t){let n=this.device.createTexture({size:[e,t],format:this.format,usage:GPUTextureUsage.TEXTURE_BINDING|GPUTextureUsage.COPY_DST|GPUTextureUsage.RENDER_ATTACHMENT});return{texture:n,view:n.createView(),width:e,height:t,sourceData:null,bindGroup:null}}render(e,t,n,r,i,a,o,s){if(!this.device||!this.context||!this.pipeline||!this._canvas)return;let c;try{let e=this.context.getCurrentTexture();if(e.width===0||e.height===0)return;c=e.createView()}catch{return}this.device.queue.writeBuffer(this.uniformBuffer,0,new Float32Array([this._canvas.width,this._canvas.height,s,0]));let l=this.device.createCommandEncoder(),u=l.beginRenderPass({colorAttachments:[{view:c,clearValue:{r:0,g:0,b:0,a:0},loadOp:`clear`,storeOp:`store`}]});for(u.setPipeline(this.pipeline);this.textures.length<e.length;)this.textures.push(this.createTextureInfo(64,64));for(;this.quadDataBuffers.length<e.length;)this.quadDataBuffers.push(this.device.createBuffer({size:32,usage:GPUBufferUsage.STORAGE|GPUBufferUsage.COPY_DST}));for(let t=0;t<e.length;t++){let{pixelData:n,x:s,y:c}=e[t],{width:l,height:d,data:f}=n;if(l<=0||d<=0)continue;let p=this.textures[t];if((p.width!==l||p.height!==d)&&(this.pendingDestroyTextures.push(p.texture),p=this.createTextureInfo(l,d),this.textures[t]=p),p.sourceData!==f){let e=new Uint8Array(f.length);if(this.format===`bgra8unorm`)for(let t=0;t<f.length;t+=4){let n=f[t+3],r=n/255;e[t]=f[t+2]*r+.5|0,e[t+1]=f[t+1]*r+.5|0,e[t+2]=f[t]*r+.5|0,e[t+3]=n}else for(let t=0;t<f.length;t+=4){let n=f[t+3],r=n/255;e[t]=f[t]*r+.5|0,e[t+1]=f[t+1]*r+.5|0,e[t+2]=f[t+2]*r+.5|0,e[t+3]=n}this.device.queue.writeTexture({texture:p.texture},e,{bytesPerRow:l*4},{width:l,height:d}),p.sourceData=f}let m=l*r,h=d*i,g=s*r+a,_=c*i+o,v=new Float32Array([g,_,m,h,l,d,0,0]),y=this.quadDataBuffers[t];this.device.queue.writeBuffer(y,0,v),p.bindGroup||(p.bindGroup=this.device.createBindGroup({layout:this.bindGroupLayout,entries:[{binding:0,resource:{buffer:this.uniformBuffer}},{binding:1,resource:{buffer:y}},{binding:2,resource:this.sampler},{binding:3,resource:p.view}]})),u.setBindGroup(0,p.bindGroup),u.draw(6)}u.end(),this.device.queue.submit([l.finish()]);for(let e of this.pendingDestroyTextures)e.destroy();if(this.pendingDestroyTextures=[],this.textures.length>e.length){for(let t=e.length;t<this.textures.length;t++)this.textures[t].texture.destroy();this.textures.length=e.length}if(this.quadDataBuffers.length>e.length){for(let t=e.length;t<this.quadDataBuffers.length;t++)this.quadDataBuffers[t].destroy();this.quadDataBuffers.length=e.length}}clear(){if(!(!this.device||!this.context))try{let e=this.context.getCurrentTexture();if(e.width===0||e.height===0)return;let t=this.device.createCommandEncoder();t.beginRenderPass({colorAttachments:[{view:e.createView(),clearValue:{r:0,g:0,b:0,a:0},loadOp:`clear`,storeOp:`store`}]}).end(),this.device.queue.submit([t.finish()])}catch{return}}async readPixels(){if(!this.device||!this.context||!this._canvas)return{data:new Uint8ClampedArray,width:0,height:0};let e=this._canvas.width,t=this._canvas.height;if(e<=0||t<=0)return{data:new Uint8ClampedArray,width:0,height:0};let n=Math.ceil(e*4/256)*256,r=n*t,i=this.device.createBuffer({size:r,usage:GPUBufferUsage.COPY_DST|GPUBufferUsage.MAP_READ});try{let r=this.context.getCurrentTexture(),a=this.device.createCommandEncoder();a.copyTextureToBuffer({texture:r},{buffer:i,bytesPerRow:n},{width:e,height:t}),this.device.queue.submit([a.finish()]),await i.mapAsync(GPUMapMode.READ);let o=new Uint8Array(i.getMappedRange()),s=new Uint8ClampedArray(e*t*4),c=this.format===`bgra8unorm`;for(let r=0;r<t;r+=1){let t=r*n;for(let n=0;n<e;n+=1){let i=t+n*4,a=(r*e+n)*4,l=o[i],u=o[i+1],d=o[i+2],f=o[i+3];if(c){let e=l;l=d,d=e}if(f>0&&f<255){let e=255/f;l=Math.min(255,Math.round(l*e)),u=Math.min(255,Math.round(u*e)),d=Math.min(255,Math.round(d*e))}s[a]=l,s[a+1]=u,s[a+2]=d,s[a+3]=f}}return{data:s,width:e,height:t}}finally{try{i.unmap()}catch{}i.destroy()}}get initialized(){return this._initialized}destroy(){for(let e of this.textures)e.texture.destroy();this.textures=[];for(let e of this.pendingDestroyTextures)e.destroy();this.pendingDestroyTextures=[],this.uniformBuffer?.destroy();for(let e of this.quadDataBuffers)e.destroy();this.quadDataBuffers=[],this.device?.destroy(),this.device=null,this.context=null,this._canvas=null,this._initialized=!1,this._initPromise=null,this._lastCanvasWidth=0,this._lastCanvasHeight=0}};function k(e){if(e.buffer instanceof ArrayBuffer)return e instanceof Uint8ClampedArray?e:new Uint8ClampedArray(e.buffer,e.byteOffset,e.byteLength);let t=new Uint8ClampedArray(e.byteLength);return t.set(e),t}function A(e,t){let n=e.length;if(n===0)return-1;let r=0,i=n-1,a=-1;for(;r<=i;){let n=r+i>>>1;e[n]<=t?(a=n,r=n+1):i=n-1}return a}function j(e){let t=e.compositions.flatMap(e=>{let t=M(e.rgba,e.width,e.height);return t?{pixelData:t.pixelData,x:e.x+t.offsetX,y:e.y+t.offsetY}:[]});return{width:e.width,height:e.height,compositionData:t}}function M(e,t,n){let r=k(e);if(t<=0||n<=0||r.length!==t*n*4)return null;let i=t,a=n,o=-1,s=-1;for(let e=3;e<r.length;e+=4){if(r[e]===0)continue;let n=e-3>>2,c=Math.floor(n/t),l=n-c*t;l<i&&(i=l),c<a&&(a=c),l>o&&(o=l),c>s&&(s=c)}if(o<i||s<a)return null;if(i===0&&a===0&&o===t-1&&s===n-1)return{pixelData:new ImageData(r,t,n),offsetX:0,offsetY:0};let c=o-i+1,l=s-a+1,u=new Uint8ClampedArray(c*l*4);for(let e=0;e<l;e++){let n=((a+e)*t+i)*4,o=n+c*4;u.set(r.subarray(n,o),e*c*4)}return{pixelData:new ImageData(u,c,l),offsetX:i,offsetY:a}}function N(e){if(e.compositionData.length===0)return null;let t=1/0,n=1/0,r=-1/0,i=-1/0;for(let a of e.compositionData)t=Math.min(t,a.x),n=Math.min(n,a.y),r=Math.max(r,a.x+a.pixelData.width),i=Math.max(i,a.y+a.pixelData.height);return!Number.isFinite(t)||!Number.isFinite(n)||!Number.isFinite(r)||!Number.isFinite(i)?null:{x:t,y:n,width:Math.max(0,r-t),height:Math.max(0,i-n)}}function P(e,t,n,r=null){for(e.frameCache.has(t)&&e.frameCache.delete(t),e.renderIssues.has(t)&&e.renderIssues.delete(t),e.frameCache.set(t,n),e.renderIssues.set(t,r);e.frameCache.size>e.cacheLimit;){let t=e.frameCache.keys().next().value;if(t===void 0)break;e.frameCache.delete(t),e.renderIssues.delete(t)}}function F(e,t){for(e.cacheLimit=Math.max(0,Math.floor(t));e.frameCache.size>e.cacheLimit;){let t=e.frameCache.keys().next().value;if(t===void 0)break;e.frameCache.delete(t),e.renderIssues.delete(t)}return e.cacheLimit}function I(){return typeof crypto<`u`&&typeof crypto.randomUUID==`function`?crypto.randomUUID():`libbitsub-${Date.now()}-${Math.random().toString(36).slice(2)}`}function L(){return{useWorker:_(),workerReady:!1,sessionId:null,timestamps:new Float64Array,frameCache:new Map,renderIssues:new Map,pendingRenders:new Map,cacheLimit:24,metadata:null}}function R(e,t,n,r){let i=n[r+3];if(i===0)return;if(i===255){e[t]=n[r],e[t+1]=n[r+1],e[t+2]=n[r+2],e[t+3]=255;return}let a=e[t+3];if(a===0){e[t]=n[r],e[t+1]=n[r+1],e[t+2]=n[r+2],e[t+3]=i;return}let o=i/255,s=a/255,c=o+s*(1-o);if(c<=0){e[t]=0,e[t+1]=0,e[t+2]=0,e[t+3]=0;return}let l=n[r]*o,u=n[r+1]*o,d=n[r+2]*o,f=e[t]*s,p=e[t+1]*s,m=e[t+2]*s;e[t]=Math.round((l+f*(1-o))/c),e[t+1]=Math.round((u+p*(1-o))/c),e[t+2]=Math.round((d+m*(1-o))/c),e[t+3]=Math.round(c*255)}function z(e,t={}){let n=t.crop??`bounds`,r=N(e);if(!r&&n===`bounds`)return null;let i=n===`screen`?0:r?.x??0,a=n===`screen`?0:r?.y??0,o=Math.max(1,n===`screen`?e.width:r?.width??1),s=Math.max(1,n===`screen`?e.height:r?.height??1),c=new Uint8ClampedArray(o*s*4);for(let t of e.compositionData){let e=t.pixelData.data,n=t.pixelData.width,r=t.pixelData.height;if(n<=0||r<=0)continue;let l=t.x-i,u=t.y-a,d=Math.max(0,l),f=Math.max(0,u),p=Math.min(o,l+n),m=Math.min(s,u+r);if(!(d>=p||f>=m))for(let t=f;t<m;t+=1){let r=t-u;for(let i=d;i<p;i+=1){let a=i-l,s=(r*n+a)*4;R(c,(t*o+i)*4,e,s)}}}return{imageData:new ImageData(c,o,s),bounds:r,offsetX:i,offsetY:a,screenWidth:e.width,screenHeight:e.height,crop:n,compositionCount:e.compositionData.length}}var B=class{parser=null;timestamps=new Float64Array;cueMetadataCache=new Map;debug;onWarning;constructor(e={}){let t=u();this.parser=new t.PgsParser,this.debug=!!e.debug,this.onWarning=e.onWarning}load(e){try{if(!this.parser)throw Error(`Parser not initialized`);let t=this.parser.parse(e);return this.timestamps=this.parser.getTimestamps(),this.cueMetadataCache.clear(),t}catch(e){throw n(e,{format:`pgs`})}}reset(){this.parser?.reset(),this.timestamps=new Float64Array,this.cueMetadataCache.clear()}feed(e){try{if(!this.parser)throw Error(`Parser not initialized`);let t=this.parser.feed(e);return(t>0||this.timestamps.length!==this.parser.count)&&(this.timestamps=this.parser.getTimestamps(),this.cueMetadataCache.clear()),t}catch(e){throw n(e,{format:`pgs`})}}finishFeed(){try{if(!this.parser)throw Error(`Parser not initialized`);let e=this.parser.finishFeed();return this.timestamps=this.parser.getTimestamps(),e}catch(e){throw n(e,{format:`pgs`})}}get pendingLen(){return this.parser?.pendingLen??0}getTimestamps(){return this.timestamps}get count(){return this.parser?.count??0}findIndexAtTimestamp(e){return this.parser?this.parser.findIndexAtTimestamp(e*1e3):-1}renderAtIndex(e){if(!this.parser)return;let t=this.parser.renderAtIndex(e);if(!t){let t=i(this.getLastRenderIssue(),{format:`pgs`,cueIndex:e});t&&this.emitWarning(t);return}return this.convertFrame(t)}getLastRenderIssue(){return this.parser?.lastRenderIssue?.trim()||null}getMetadata(){return{format:`pgs`,cueCount:this.count,screenWidth:this.parser?.screenWidth??0,screenHeight:this.parser?.screenHeight??0}}getCueMetadata(e){if(!this.parser||e<0||e>=this.count)return null;if(this.cueMetadataCache.has(e))return this.cueMetadataCache.get(e)??null;let t=this.parser.getCueStartTime(e),n=this.parser.getCueEndTime(e),r=this.renderAtIndex(e),i={index:e,format:`pgs`,startTime:t,endTime:n,duration:Math.max(0,n-t),screenWidth:this.parser.screenWidth,screenHeight:this.parser.screenHeight,bounds:r?N(r):null,compositionCount:this.parser.getCueCompositionCount(e),paletteId:this.parser.getCuePaletteId(e),compositionState:this.parser.getCueCompositionState(e)};return this.cueMetadataCache.set(e,i),i}renderAtTimestamp(e){let t=this.findIndexAtTimestamp(e);if(!(t<0))return this.renderAtIndex(t)}renderFrameDataAtIndex(e,t={}){let n=this.renderAtIndex(e);return n?z(n,t)??void 0:void 0}renderFrameDataAtTimestamp(e,t={}){let n=this.renderAtTimestamp(e);return n?z(n,t)??void 0:void 0}convertFrame(e){let t=[];for(let n=0;n<e.compositionCount;n++){let i=e.getComposition(n);if(!i)continue;let a=i.getRgba(),o=i.width*i.height*4;if(a.length!==o||i.width===0||i.height===0){this.emitWarning(r(`INVALID_FRAME_DATA`,`Invalid PGS composition buffer dimensions during frame conversion.`,{format:`pgs`,details:{expectedLength:o,actualLength:a.length,width:i.width,height:i.height}}));continue}let s=M(a,i.width,i.height);s&&t.push({pixelData:s.pixelData,x:i.x+s.offsetX,y:i.y+s.offsetY})}return{width:e.width,height:e.height,compositionData:t}}clearCache(){this.parser?.clearCache(),this.cueMetadataCache.clear()}dispose(){this.parser?.free(),this.parser=null,this.timestamps=new Float64Array,this.cueMetadataCache.clear()}emitWarning(e){this.onWarning?.(e),this.debug&&!this.onWarning&&console.warn(a(e),e.details??{})}},V=2097152,H=524288;function U(e,t,n,r,i){e&&e({loaded:t,total:n,ratio:n&&n>0?Math.min(1,t/n):null,rangeSupported:r,strategy:i})}function W(e){if(!e)return null;let t=Number(e);return Number.isFinite(t)&&t>=0?t:null}function G(e){if(!e)return null;let t=/bytes\s+(?:\d+-\d+|\*)\/(\d+|\*)/i.exec(e);return!t||t[1]===`*`?null:W(t[1])}function K(e,t){let n=new Headers(e);return t&&new Headers(t).forEach((e,t)=>n.set(t,e)),n}async function q(e,t={}){let n=K(t.headers,{Range:`bytes=0-0`});try{let r=await fetch(e,{method:`GET`,headers:n,signal:t.signal});if(r.status===206){let e=G(r.headers.get(`content-range`))??W(r.headers.get(`content-length`));try{await r.body?.cancel()}catch{}return{supportsRange:!0,size:e,acceptRanges:r.headers.get(`accept-ranges`)}}if(r.ok){let e=r.headers.get(`accept-ranges`),t=W(r.headers.get(`content-length`));try{await r.body?.cancel()}catch{}return{supportsRange:!!(e&&e.toLowerCase()!==`none`),size:t,acceptRanges:e}}}catch{}try{let n=await fetch(e,{method:`HEAD`,headers:K(t.headers),signal:t.signal});if(!n.ok)return{supportsRange:!1,size:null,acceptRanges:null};let r=n.headers.get(`accept-ranges`);return{supportsRange:!!(r&&r.toLowerCase()!==`none`),size:W(n.headers.get(`content-length`)),acceptRanges:r}}catch{return{supportsRange:!1,size:null,acceptRanges:null}}}async function J(e,t,n,r,i,a){let o=t??W(e.headers.get(`content-length`));if(!e.body||typeof e.body.getReader!=`function`){let t=new Uint8Array(await e.arrayBuffer()),s={loaded:t.byteLength,total:o??t.byteLength,ratio:1,rangeSupported:n,strategy:r===`stream`?`basic`:r};return await a?.(t,s),i?.(s),t}let s=e.body.getReader(),c=[],l=0;for(;;){let{done:e,value:t}=await s.read();if(e)break;if(!t||t.byteLength===0)continue;let u=t instanceof Uint8Array?t:new Uint8Array(t);c.push(u),l+=u.byteLength;let d={loaded:l,total:o,ratio:o&&o>0?Math.min(1,l/o):null,rangeSupported:n,strategy:r};await a?.(u,d),i?.(d)}let u=new Uint8Array(l),d=0;for(let e of c)u.set(e,d),d+=e.byteLength;return U(i,u.byteLength,o??u.byteLength,n,r),u}async function ee(e,t,n,r){let i=Math.max(1,Math.floor(n.rangeChunkSize??H)),a=new Uint8Array(t),o=0;for(let s=0;s<t;s+=i){let c=Math.min(t-1,s+i-1),l=await fetch(e,{method:`GET`,headers:K(n.headers,{Range:`bytes=${s}-${c}`}),signal:n.signal});if(l.status!==206&&!(l.ok&&s===0&&c>=t-1))throw Error(`Failed to fetch subtitle range ${s}-${c}: ${l.status}`);let u=new Uint8Array(await l.arrayBuffer());if(u.byteLength===0)throw Error(`Empty subtitle range response for bytes=${s}-${c}`);s+u.byteLength>t?(a.set(u.subarray(0,t-s),s),o=t):(a.set(u,s),o=s+u.byteLength);let d=a.subarray(s,o),f={loaded:o,total:t,ratio:t>0?Math.min(1,o/t):null,rangeSupported:!0,strategy:`range-chunks`};await r?.(d,f),n.onProgress?.(f)}return a}async function Y(e,t={},n){let r=t.preferRange!==!1,i=t.rangeChunkThreshold??V,a=!1,o=null;if(r){let r=await q(e,t);if(a=r.supportsRange,o=r.size,a&&o!=null&&o>=i)return{data:await ee(e,o,t,n),strategy:`range-chunks`,rangeSupported:!0,total:o}}let s=await fetch(e,{method:`GET`,headers:K(t.headers),signal:t.signal});if(!s.ok)throw Error(`Failed to fetch subtitle: ${s.status}`);let c=o??W(s.headers.get(`content-length`)),l=s.body?`stream`:`basic`,u=await J(s,c,a,l,t.onProgress,n);return{data:u,strategy:l,rangeSupported:a,total:c??u.byteLength}}var te=`#version 300 es

uniform vec2 u_resolution;
uniform vec4 u_destRect; // x, y, w, h in pixels

out vec2 v_texCoord;

void main() {
  // Generate unit-square positions for two-triangle quad (CCW)
  vec2 unitPos;
  if (gl_VertexID == 0) unitPos = vec2(0.0, 0.0);
  else if (gl_VertexID == 1) unitPos = vec2(1.0, 0.0);
  else if (gl_VertexID == 2) unitPos = vec2(0.0, 1.0);
  else if (gl_VertexID == 3) unitPos = vec2(1.0, 0.0);
  else if (gl_VertexID == 4) unitPos = vec2(1.0, 1.0);
  else                       unitPos = vec2(0.0, 1.0);

  v_texCoord = unitPos;

  // Convert pixel position to clip space
  vec2 pixelPos = u_destRect.xy + unitPos * u_destRect.zw;
  vec2 clipPos = (pixelPos / u_resolution) * 2.0 - 1.0;
  clipPos.y = -clipPos.y; // Flip Y for canvas coordinates

  gl_Position = vec4(clipPos, 0.0, 1.0);
}
`,ne=`#version 300 es
precision mediump float;

uniform sampler2D u_texture;
uniform float u_opacity;

in vec2 v_texCoord;
out vec4 outColor;

void main() {
  // Texture is pre-multiplied alpha; output as-is for premultiplied blending
  outColor = texture(u_texture, v_texCoord) * u_opacity;
}
`,X=null;function Z(){if(X!==null)return X;if(typeof document>`u`)return X=!1;try{X=!!document.createElement(`canvas`).getContext(`webgl2`)}catch{X=!1}return X}var re=class{gl=null;program=null;vao=null;uResolution=null;uDestRect=null;uTexture=null;uOpacity=null;textures=[];_canvas=null;_initialized=!1;_width=0;_height=0;async init(){}async setCanvas(e,t,n){this._canvas=e,e.width=t,e.height=n,this._width=t,this._height=n;let r=e.getContext(`webgl2`,{alpha:!0,premultipliedAlpha:!0,antialias:!1,depth:!1,stencil:!1});if(!r)throw Error(`Could not get WebGL2 context`);this.gl=r;let i=this._compileShader(r.VERTEX_SHADER,te),a=this._compileShader(r.FRAGMENT_SHADER,ne),o=r.createProgram();if(!o)throw Error(`Failed to create WebGL2 program`);if(r.attachShader(o,i),r.attachShader(o,a),r.linkProgram(o),r.deleteShader(i),r.deleteShader(a),!r.getProgramParameter(o,r.LINK_STATUS)){let e=r.getProgramInfoLog(o);throw r.deleteProgram(o),Error(`WebGL2 program link failed: `+e)}this.program=o,r.useProgram(o),this.uResolution=r.getUniformLocation(o,`u_resolution`),this.uDestRect=r.getUniformLocation(o,`u_destRect`),this.uTexture=r.getUniformLocation(o,`u_texture`),this.uOpacity=r.getUniformLocation(o,`u_opacity`),this.vao=r.createVertexArray(),r.bindVertexArray(this.vao),r.uniform2f(this.uResolution,t,n),r.uniform1i(this.uTexture,0),r.uniform1f(this.uOpacity,1),r.enable(r.BLEND),r.blendFunc(r.ONE,r.ONE_MINUS_SRC_ALPHA),r.viewport(0,0,t,n),this._initialized=!0,console.log(`[libbitsub] WebGL2 renderer initialized`)}_compileShader(e,t){let n=this.gl,r=n.createShader(e);if(!r)throw Error(`Failed to create shader`);if(n.shaderSource(r,t),n.compileShader(r),!n.getShaderParameter(r,n.COMPILE_STATUS)){let e=n.getShaderInfoLog(r);throw n.deleteShader(r),Error(`Shader compile error: `+e)}return r}updateSize(e,t){!this.gl||!this._canvas||(this._canvas.width=e,this._canvas.height=t,this._width=e,this._height=t,this.gl.viewport(0,0,e,t),this.gl.useProgram(this.program),this.gl.uniform2f(this.uResolution,e,t))}_ensureTexture(e,t,n){let r=this.gl,i=this.textures[e];if(i&&i.width===t&&i.height===n)return i;i&&r.deleteTexture(i.texture);let a=r.createTexture();if(!a)throw Error(`Failed to create WebGL2 texture`);r.bindTexture(r.TEXTURE_2D,a),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MIN_FILTER,r.LINEAR),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_MAG_FILTER,r.LINEAR),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_WRAP_S,r.CLAMP_TO_EDGE),r.texParameteri(r.TEXTURE_2D,r.TEXTURE_WRAP_T,r.CLAMP_TO_EDGE),r.texImage2D(r.TEXTURE_2D,0,r.RGBA8,t,n,0,r.RGBA,r.UNSIGNED_BYTE,null);let o={texture:a,width:t,height:n,sourceData:null};return this.textures[e]=o,o}render(e,t,n,r,i,a,o,s){if(!this.gl||!this.program||!this._canvas)return;let c=this.gl;c.useProgram(this.program),c.bindVertexArray(this.vao),c.clearColor(0,0,0,0),c.clear(c.COLOR_BUFFER_BIT),c.activeTexture(c.TEXTURE0),c.uniform1f(this.uOpacity,s);for(let t=0;t<e.length;t++){let{pixelData:n,x:s,y:l}=e[t],{width:u,height:d,data:f}=n;if(u<=0||d<=0)continue;let p=this._ensureTexture(t,u,d);if(c.bindTexture(c.TEXTURE_2D,p.texture),p.sourceData!==f){let e=new Uint8Array(f.length);for(let t=0;t<f.length;t+=4){let n=f[t+3],r=n/255;e[t]=f[t]*r+.5|0,e[t+1]=f[t+1]*r+.5|0,e[t+2]=f[t+2]*r+.5|0,e[t+3]=n}c.texSubImage2D(c.TEXTURE_2D,0,0,0,u,d,c.RGBA,c.UNSIGNED_BYTE,e),p.sourceData=f}let m=u*r,h=d*i,g=s*r+a,_=l*i+o;c.uniform4f(this.uDestRect,g,_,m,h),c.drawArrays(c.TRIANGLES,0,6)}if(this.textures.length>e.length){for(let t=e.length;t<this.textures.length;t++){let e=this.textures[t];e&&c.deleteTexture(e.texture)}this.textures.length=e.length}}clear(){this.gl&&(this.gl.clearColor(0,0,0,0),this.gl.clear(this.gl.COLOR_BUFFER_BIT))}readPixels(){if(!this.gl||!this._canvas)return{data:new Uint8ClampedArray,width:0,height:0};let e=this._canvas.width,t=this._canvas.height,n=this.gl,r=new Uint8Array(e*t*4);n.pixelStorei(n.PACK_ALIGNMENT,1),n.readPixels(0,0,e,t,n.RGBA,n.UNSIGNED_BYTE,r);let i=new Uint8ClampedArray(r.length),a=e*4;for(let e=0;e<t;e+=1){let n=(t-1-e)*a,o=e*a;i.set(r.subarray(n,n+a),o)}for(let e=0;e<i.length;e+=4){let t=i[e+3];if(t>0&&t<255){let n=255/t;i[e]=Math.min(255,Math.round(i[e]*n)),i[e+1]=Math.min(255,Math.round(i[e+1]*n)),i[e+2]=Math.min(255,Math.round(i[e+2]*n))}}return{data:i,width:e,height:t}}get initialized(){return this._initialized}destroy(){let e=this.gl;if(e){for(let t of this.textures)t&&e.deleteTexture(t.texture);this.program&&e.deleteProgram(this.program),this.vao&&e.deleteVertexArray(this.vao)}this.textures=[],this.program=null,this.vao=null,this.gl=null,this._canvas=null,this._initialized=!1}},Q={scale:1,aspectMode:`stretch`,verticalOffset:0,horizontalOffset:0,horizontalAlign:`center`,bottomPadding:0,safeArea:0,opacity:1};function $(e,t){return!t&&e.buffer instanceof ArrayBuffer&&e.byteOffset===0&&e.byteLength===e.buffer.byteLength?e.buffer:e.slice().buffer}var ie=class{video;format;subUrl;subContent;canvas=null;ctx=null;animationFrameId=null;isLoaded=!1;lastRenderedIndex=-1;lastRenderedTime=-1;disposed=!1;resizeObserver=null;tempCanvas=null;tempCtx=null;lastRenderedData=null;lastCueIndex=null;currentCueMetadata=null;parserMetadata=null;displaySettings={...Q};timeOffset=0;cacheLimit=24;prefetchBefore=0;prefetchAfter=0;streamingLoad=!0;rangeRequests=!0;onEvent;onWarning;currentRendererBackend=null;debug;lastRenderInfo=null;loadedMetadataHandler=null;seekedHandler=null;webgpuRenderer=null;useWebGPU=!1;onWebGPUFallback;webgl2Renderer=null;useWebGL2=!1;onWebGL2Fallback;perfStats={framesRendered:0,framesDropped:0,renderTimes:[],lastRenderTime:0,fpsTimestamps:[],lastFrameTime:0};constructor(e,t){this.video=e.video,this.format=t,this.subUrl=e.subUrl,this.subContent=e.subContent,this.onWebGPUFallback=e.onWebGPUFallback,this.onWebGL2Fallback=e.onWebGL2Fallback,this.onEvent=e.onEvent,this.onWarning=e.onWarning,this.debug=!!e.debug,this.displaySettings={...Q,...e.displaySettings},this.timeOffset=e.timeOffset??0,this.cacheLimit=Math.max(0,Math.floor(e.cacheLimit??24)),this.prefetchBefore=Math.max(0,Math.floor(e.prefetchWindow?.before??0)),this.prefetchAfter=Math.max(0,Math.floor(e.prefetchWindow?.after??0)),this.streamingLoad=e.streamingLoad!==!1,this.rangeRequests=e.rangeRequests!==!1}emitLoadProgress(e,t,n){this.emitEvent({type:`load-progress`,format:e,loadedBytes:t.loaded,totalBytes:t.total,ratio:t.ratio,strategy:t.strategy,rangeSupported:t.rangeSupported,indexedCues:n})}emitIndexed(e,t,n){this.emitEvent({type:`indexed`,format:e,metadata:t,partial:n})}memoryProgress(e){return{loaded:e,total:e,ratio:1,rangeSupported:!1,strategy:`memory`}}getDisplaySettings(){return{...this.displaySettings}}getCacheStats(){let e=this.getWorkerRendererState();return{cacheLimit:this.cacheLimit,cachedFrames:e.frameCache.size,pendingRenders:e.pendingRenders.size,totalEntries:e.timestamps.length,usingWorker:e.useWorker&&e.workerReady,workerReady:e.workerReady,sessionId:e.sessionId}}getLastRenderInfo(){return this.lastRenderInfo?{...this.lastRenderInfo,cache:{...this.lastRenderInfo.cache},cue:this.lastRenderInfo.cue?{...this.lastRenderInfo.cue}:null}:null}getMetadata(){return this.parserMetadata}getCurrentCueMetadata(){return this.currentCueMetadata}getCueMetadata(e){return this.buildCueMetadata(e)}getCacheLimit(){return this.cacheLimit}getBaseStats(){let e=performance.now();this.perfStats.fpsTimestamps=this.perfStats.fpsTimestamps.filter(t=>e-t<1e3);let t=this.perfStats.renderTimes,n=t.length>0?t.reduce((e,t)=>e+t,0)/t.length:0,r=t.length>0?Math.max(...t):0,i=t.length>0?Math.min(...t):0;return{framesRendered:this.perfStats.framesRendered,framesDropped:this.perfStats.framesDropped,avgRenderTime:Math.round(n*100)/100,maxRenderTime:Math.round(r*100)/100,minRenderTime:Math.round(i*100)/100,lastRenderTime:Math.round(this.perfStats.lastRenderTime*100)/100,renderFps:this.perfStats.fpsTimestamps.length,currentIndex:this.lastRenderedIndex}}setDisplaySettings(e){let t={...this.displaySettings,...e};t.scale=Math.max(.1,Math.min(3,t.scale)),[`stretch`,`contain`,`cover`].includes(t.aspectMode)||(t.aspectMode=Q.aspectMode),t.verticalOffset=Math.max(-50,Math.min(50,t.verticalOffset)),t.horizontalOffset=Math.max(-50,Math.min(50,t.horizontalOffset)),t.bottomPadding=Math.max(0,Math.min(50,t.bottomPadding)),t.safeArea=Math.max(0,Math.min(25,t.safeArea)),t.opacity=Math.max(0,Math.min(1,t.opacity));let n=JSON.stringify(t)!==JSON.stringify(this.displaySettings);this.displaySettings=t,n&&(this.lastRenderedIndex=-1,this.lastRenderedTime=-1)}resetDisplaySettings(){this.displaySettings={...Q},this.lastRenderedIndex=-1,this.lastRenderedTime=-1}startInit(){this.init().catch(e=>{this.emitEvent({type:`error`,format:this.format,error:n(e,{format:this.format})})})}async init(){await l(),this.createCanvas(),await new Promise(e=>setTimeout(e,0)),await this.loadSubtitles(),this.startRenderLoop()}createCanvas(){this.canvas=document.createElement(`canvas`),Object.assign(this.canvas.style,{position:`absolute`,pointerEvents:`none`,zIndex:`10`});let e=this.video.parentElement;e&&(window.getComputedStyle(e).position===`static`&&(e.style.position=`relative`),e.appendChild(this.canvas)),D()?this.initWebGPU():Z()?this.initWebGL2():this.initCanvas2D(),this.updateCanvasSize(),this.resizeObserver=new ResizeObserver(()=>this.updateCanvasSize()),this.resizeObserver.observe(this.video),this.loadedMetadataHandler=()=>this.updateCanvasSize(),this.seekedHandler=()=>{this.lastRenderedIndex=-1,this.lastRenderedTime=-1,this.onSeek()},this.video.addEventListener(`loadedmetadata`,this.loadedMetadataHandler),this.video.addEventListener(`seeked`,this.seekedHandler)}emitEvent(e){this.onEvent?.(e)}emitWarning(e){this.onWarning?.(e),this.emitEvent({type:`warning`,warning:e}),this.debug&&!this.onWarning&&console.warn(a(e),e.details??{})}setParserMetadata(e){this.parserMetadata=e,e&&this.emitEvent({type:`loaded`,format:this.format,metadata:e})}emitWorkerState(e,t,n,r=!1){this.emitEvent({type:`worker-state`,enabled:e,ready:t,sessionId:n,fallback:r})}emitCacheChange(e,t){this.emitEvent({type:`cache-change`,cachedFrames:e,pendingRenders:t,cacheLimit:this.cacheLimit})}emitCueChange(e){if(this.lastCueIndex===e?.index&&e?.index!==void 0){this.currentCueMetadata=e;return}this.lastCueIndex=e?.index??null,this.currentCueMetadata=e,this.emitEvent({type:`cue-change`,cue:e})}emitRendererBackend(e){this.currentRendererBackend!==e&&(this.currentRendererBackend=e,this.emitEvent({type:`renderer-change`,renderer:e}))}recordLastRenderInfo(e){this.debug&&(this.lastRenderInfo=e)}async initWebGPU(){try{if(this.webgpuRenderer=new O,await this.webgpuRenderer.init(),!this.canvas)return;let e=this.getVideoContentBounds(),t=Math.max(1,e.width*window.devicePixelRatio),n=Math.max(1,e.height*window.devicePixelRatio);await this.webgpuRenderer.setCanvas(this.canvas,t,n),this.useWebGPU=!0,this.emitRendererBackend(`webgpu`)}catch{this.webgpuRenderer?.destroy(),this.webgpuRenderer=null,this.useWebGPU=!1,this.onWebGPUFallback?.(),Z()?this.initWebGL2():this.initCanvas2D()}}async initWebGL2(){try{if(this.webgl2Renderer=new re,await this.webgl2Renderer.init(),!this.canvas)return;let e=this.getVideoContentBounds(),t=Math.max(1,e.width*window.devicePixelRatio),n=Math.max(1,e.height*window.devicePixelRatio);await this.webgl2Renderer.setCanvas(this.canvas,t,n),this.useWebGL2=!0,this.emitRendererBackend(`webgl2`)}catch{this.webgl2Renderer?.destroy(),this.webgl2Renderer=null,this.useWebGL2=!1,this.onWebGL2Fallback?.(),this.initCanvas2D()}}initCanvas2D(){this.canvas&&(this.ctx=this.canvas.getContext(`2d`),this.useWebGPU=!1,this.useWebGL2=!1,this.emitRendererBackend(`canvas2d`))}onSeek(){}getVideoContentBounds(){let e=this.video.getBoundingClientRect(),t=this.video.videoWidth||e.width,n=this.video.videoHeight||e.height,r=e.width/e.height,i=t/n,a,o,s,c;return Math.abs(r-i)<.01?(a=e.width,o=e.height,s=0,c=0):r>i?(o=e.height,a=e.height*i,s=(e.width-a)/2,c=0):(a=e.width,o=e.width/i,s=0,c=(e.height-o)/2),{x:s,y:c,width:a,height:o}}updateCanvasSize(){if(!this.canvas)return;let e=this.getVideoContentBounds(),t=e.width>0?e.width:this.video.videoWidth||1920,n=e.height>0?e.height:this.video.videoHeight||1080,r=Math.max(1,t*window.devicePixelRatio),i=Math.max(1,n*window.devicePixelRatio);this.canvas.width=r,this.canvas.height=i,this.canvas.style.left=`${e.x}px`,this.canvas.style.top=`${e.y}px`,this.canvas.style.width=`${e.width}px`,this.canvas.style.height=`${e.height}px`,this.useWebGPU&&this.webgpuRenderer?this.webgpuRenderer.updateSize(r,i):this.useWebGL2&&this.webgl2Renderer&&this.webgl2Renderer.updateSize(r,i),this.lastRenderedIndex=-1,this.lastRenderedTime=-1}startRenderLoop(){this.tempCanvas=document.createElement(`canvas`),this.tempCtx=this.tempCanvas.getContext(`2d`);let e=()=>{if(!this.disposed){if(this.isLoaded){let e=this.video.currentTime+this.timeOffset,t=this.findCurrentIndex(e);if(t!==this.lastRenderedIndex){let n=t>=0&&this.getWorkerRendererState().frameCache.has(t),r=performance.now(),i=this.renderFrame(e,t),a=performance.now(),o=a-r;this.perfStats.lastRenderTime=o,this.perfStats.renderTimes.push(o),this.perfStats.renderTimes.length>60&&this.perfStats.renderTimes.shift(),this.perfStats.framesRendered++,this.perfStats.fpsTimestamps.push(a),o>16.67&&this.perfStats.framesDropped++,i.warning&&this.emitWarning(i.warning),this.lastRenderedIndex=t,this.lastRenderedTime=e;let s=t>=0?this.buildCueMetadata(t):null;this.emitCueChange(s),this.recordLastRenderInfo({time:e,index:t,status:i.status,backend:this.currentRendererBackend,usingWorker:this.getCacheStats().usingWorker,cacheHit:n,renderDuration:Math.round(o*100)/100,frameWidth:i.data?.width??null,frameHeight:i.data?.height??null,compositionCount:i.data?.compositionData.length??0,cue:s,cache:this.getCacheStats(),capturedAt:a}),this.emitEvent({type:`stats`,stats:this.getStats()}),t>=0&&(this.prefetchBefore>0||this.prefetchAfter>0)&&this.prefetchAroundTime?.call(this,e).catch(()=>{})}}this.animationFrameId=requestAnimationFrame(e)}};this.animationFrameId=requestAnimationFrame(e)}renderFrame(e,t){if(!this.canvas)return{status:`failed`,data:null,warning:null};let n=t>=0?this.renderAtIndex(t):void 0;if(n===void 0&&this.lastRenderedData!==null&&t>=0&&this.isPendingRender(t))return{status:`pending`,data:null,warning:null};let r=i(t>=0?this.getWorkerRendererState().renderIssues.get(t)??null:null,{format:this.format,cueIndex:t});return this.useWebGPU&&this.webgpuRenderer?this.renderFrameWebGPU(n,t):this.useWebGL2&&this.webgl2Renderer?this.renderFrameWebGL2(n,t):this.renderFrameCanvas2D(n,t),t<0?{status:`cleared`,data:null,warning:null}:r?{status:`failed`,data:n??null,warning:r}:!n||n.compositionData.length===0?{status:`empty`,data:n??null,warning:null}:{status:`rendered`,data:n,warning:null}}computeLayout(e){if(!this.canvas)return{scaleX:1,scaleY:1,shiftX:0,shiftY:0,opacity:this.displaySettings.opacity};let t=e.width>0?e.width:this.canvas.width,n=e.height>0?e.height:this.canvas.height,r=this.canvas.width/t,i=this.canvas.height/n,a=N(e)??{x:0,y:0,width:t,height:n},{scale:o,aspectMode:s,verticalOffset:c,horizontalOffset:l,horizontalAlign:u,bottomPadding:d,safeArea:f,opacity:p}=this.displaySettings,m=r,h=i,g=0,_=0;if(s!==`stretch`){let e=s===`cover`?Math.max(r,i):Math.min(r,i);m=e,h=e,g=(this.canvas.width-t*e)/2,_=(this.canvas.height-n*e)/2}let v=u===`left`?a.x:u===`right`?a.x+a.width:a.x+a.width/2,y=a.y+a.height,b=m*o,x=h*o,S=g+v*m*(1-o),C=_+y*h*(1-o),w=S+l/100*this.canvas.width,T=C+c/100*this.canvas.height;T-=d/100*this.canvas.height;let E=f/100*this.canvas.width,D=f/100*this.canvas.height,O=a.x*b+w,k=a.y*x+T,A=(a.x+a.width)*b+w,j=(a.y+a.height)*x+T;return O<E&&(w+=E-O),A>this.canvas.width-E&&(w-=A-(this.canvas.width-E)),k<D&&(T+=D-k),j>this.canvas.height-D&&(T-=j-(this.canvas.height-D)),{scaleX:b,scaleY:x,shiftX:w,shiftY:T,opacity:p}}renderFrameWebGPU(e,t){if(!this.webgpuRenderer||!this.canvas)return;if(t<0||!e||e.compositionData.length===0){this.webgpuRenderer.clear(),this.lastRenderedData=null;return}this.lastRenderedData=e;let n=this.computeLayout(e);this.webgpuRenderer.render(e.compositionData,e.width,e.height,n.scaleX,n.scaleY,n.shiftX,n.shiftY,n.opacity)}renderFrameWebGL2(e,t){if(!this.webgl2Renderer||!this.canvas)return;if(t<0||!e||e.compositionData.length===0){this.webgl2Renderer.clear(),this.lastRenderedData=null;return}this.lastRenderedData=e;let n=this.computeLayout(e);this.webgl2Renderer.render(e.compositionData,e.width,e.height,n.scaleX,n.scaleY,n.shiftX,n.shiftY,n.opacity)}renderFrameCanvas2D(e,t){if(!this.ctx||!this.canvas)return;if(this.ctx.clearRect(0,0,this.canvas.width,this.canvas.height),t<0||!e||e.compositionData.length===0){this.lastRenderedData=null;return}this.lastRenderedData=e;let n=this.computeLayout(e);this.ctx.save(),this.ctx.globalAlpha=n.opacity;for(let t of e.compositionData){if(!this.tempCanvas||!this.tempCtx)continue;(this.tempCanvas.width!==t.pixelData.width||this.tempCanvas.height!==t.pixelData.height)&&(this.tempCanvas.width=t.pixelData.width,this.tempCanvas.height=t.pixelData.height),this.tempCtx.putImageData(t.pixelData,0,0);let e=t.pixelData.width*n.scaleX,r=t.pixelData.height*n.scaleY,i=t.x*n.scaleX+n.shiftX,a=t.y*n.scaleY+n.shiftY;this.ctx.drawImage(this.tempCanvas,i,a,e,r)}this.ctx.restore()}dispose(){this.disposed=!0,this.animationFrameId!==null&&(cancelAnimationFrame(this.animationFrameId),this.animationFrameId=null),this.resizeObserver?.disconnect(),this.resizeObserver=null,this.loadedMetadataHandler&&=(this.video.removeEventListener(`loadedmetadata`,this.loadedMetadataHandler),null),this.seekedHandler&&=(this.video.removeEventListener(`seeked`,this.seekedHandler),null),this.webgpuRenderer&&=(this.webgpuRenderer.destroy(),null),this.webgl2Renderer&&=(this.webgl2Renderer.destroy(),null),this.canvas?.parentElement?.removeChild(this.canvas),this.canvas=null,this.ctx=null,this.tempCanvas=null,this.tempCtx=null,this.lastRenderedData=null,this.currentCueMetadata=null,this.parserMetadata=null,this.lastRenderInfo=null,this.useWebGPU=!1,this.useWebGL2=!1}},ae=class extends ie{pgsParser=null;state=L();onLoading;onLoaded;onError;constructor(e){super(e,`pgs`),this.onLoading=e.onLoading,this.onLoaded=e.onLoaded,this.onError=e.onError,F(this.state,this.cacheLimit),this.startInit()}async loadSubtitles(){try{if(this.emitEvent({type:`loading`,format:`pgs`}),this.onLoading?.(),this.subContent){let e=new Uint8Array(this.subContent);this.emitLoadProgress(`pgs`,this.memoryProgress(e.byteLength),0),await this.loadPgsBuffer(e,!0),this.onLoaded?.();return}if(!this.subUrl)throw Error(`No subtitle content or URL provided`);if(!this.streamingLoad){let{data:e,strategy:t,rangeSupported:n,total:r}=await Y(this.subUrl,{preferRange:this.rangeRequests,onProgress:e=>this.emitLoadProgress(`pgs`,e,this.state.timestamps.length)});this.emitLoadProgress(`pgs`,{loaded:e.byteLength,total:r??e.byteLength,ratio:1,rangeSupported:n,strategy:t},0),await this.loadPgsBuffer(e,!1),this.onLoaded?.();return}await this.loadPgsStreaming(this.subUrl),this.onLoaded?.()}catch(e){let t=n(e,{format:`pgs`});this.emitEvent({type:`error`,format:`pgs`,error:t}),this.onError?.(t)}}applyPgsIndexState(e,t,n,r){this.state.metadata=e,this.state.timestamps=t,this.setParserMetadata(e),this.emitIndexed(`pgs`,e,n),!this.isLoaded&&t.length>0&&(this.isLoaded=!0,r&&(this.state.workerReady=!0,this.emitWorkerState(!0,!0,this.state.sessionId)))}async loadPgsBuffer(e,t){if(this.state.useWorker)try{this.state.sessionId=I(),await y(),this.emitWorkerState(!0,!1,this.state.sessionId);let n=$(e,t),r=await S({type:`loadPgs`,sessionId:this.state.sessionId,data:n});if(r.type===`pgsLoaded`){this.state.workerReady=!0,this.state.metadata=r.metadata,this.state.timestamps=r.timestamps,this.isLoaded=!0,this.setParserMetadata(r.metadata),this.emitIndexed(`pgs`,r.metadata,!1),this.emitWorkerState(!0,!0,this.state.sessionId);return}if(r.type===`error`)throw Error(r.message)}catch(e){this.state.useWorker=!1,this.emitWorkerState(!1,!1,this.state.sessionId,!0),this.emitWarning(r(`WORKER_FALLBACK`,`PGS worker initialization failed, falling back to main-thread rendering.`,{format:`pgs`,details:{reason:e instanceof Error?e.message:String(e)}}))}await this.loadOnMainThread(e)}async loadPgsStreaming(e){let t=!1,n=!1;if(this.state.useWorker)try{this.state.sessionId=I(),await y(),this.emitWorkerState(!0,!1,this.state.sessionId);let e=await S({type:`beginPgs`,sessionId:this.state.sessionId});if(e.type===`error`)throw Error(e.message);t=!0}catch(e){this.state.useWorker=!1,t=!1,this.emitWorkerState(!1,!1,this.state.sessionId,!0),this.emitWarning(r(`WORKER_FALLBACK`,`PGS worker initialization failed, falling back to main-thread rendering.`,{format:`pgs`,details:{reason:e instanceof Error?e.message:String(e)}}))}t||(await this.yieldToMain(),this.pgsParser=new B({debug:this.debug,onWarning:e=>this.emitWarning(e)}),this.pgsParser.reset());try{let{data:r,strategy:i,rangeSupported:a,total:o}=await Y(e,{preferRange:this.rangeRequests,onProgress:e=>this.emitLoadProgress(`pgs`,e,this.state.timestamps.length)},async(e,r)=>{if(e.byteLength!==0){if(t&&this.state.sessionId){let t=$(e,!0),r=await S({type:`appendPgs`,sessionId:this.state.sessionId,data:t});if(r.type===`pgsProgress`)r.added>0||!n?(this.applyPgsIndexState(r.metadata,r.timestamps,!0,!0),n=!0):(this.state.timestamps=r.timestamps,this.state.metadata=r.metadata);else if(r.type===`error`)throw Error(r.message)}else if(this.pgsParser&&(this.pgsParser.feed(e)>0||!n)){let e=this.pgsParser.getMetadata();this.applyPgsIndexState(e,this.pgsParser.getTimestamps(),!0,!1),n=!0}this.emitLoadProgress(`pgs`,r,this.state.timestamps.length)}});if(t&&this.state.sessionId){let e=await S({type:`finishPgs`,sessionId:this.state.sessionId});if(e.type===`pgsProgress`)this.applyPgsIndexState(e.metadata,e.timestamps,!1,!0),this.state.workerReady=!0,this.isLoaded=!0,this.emitWorkerState(!0,!0,this.state.sessionId);else if(e.type===`error`)throw Error(e.message)}else if(this.pgsParser){this.pgsParser.finishFeed();let e=this.pgsParser.getMetadata();this.state.timestamps=this.pgsParser.getTimestamps(),this.state.metadata=e,this.isLoaded=!0,this.setParserMetadata(e),this.emitIndexed(`pgs`,e,!1),e.cueCount===0&&this.state.renderIssues.set(-1,`INVALID_SUBTITLE_DATA`)}this.emitLoadProgress(`pgs`,{loaded:r.byteLength,total:o??r.byteLength,ratio:1,rangeSupported:a,strategy:i},this.state.timestamps.length)}catch(n){t&&(this.state.useWorker=!1,this.emitWorkerState(!1,!1,this.state.sessionId,!0)),this.emitWarning(r(`RANGE_FALLBACK`,`Progressive PGS load failed; retrying with a full buffer fetch.`,{format:`pgs`,details:{reason:n instanceof Error?n.message:String(n)}}));let{data:i}=await Y(e,{preferRange:this.rangeRequests});await this.loadPgsBuffer(i,!1)}}async loadOnMainThread(e){await this.yieldToMain(),this.pgsParser=new B({debug:this.debug,onWarning:e=>this.emitWarning(e)}),await new Promise(t=>{(typeof requestIdleCallback<`u`?e=>requestIdleCallback(()=>e(),{timeout:1e3}):e=>setTimeout(e,0))(()=>{let n=this.pgsParser.load(e);this.state.timestamps=this.pgsParser.getTimestamps(),this.state.metadata=this.pgsParser.getMetadata(),this.isLoaded=!0,this.setParserMetadata(this.state.metadata),this.emitIndexed(`pgs`,this.state.metadata,!1),n===0&&this.state.renderIssues.set(-1,`INVALID_SUBTITLE_DATA`),t()})})}getWorkerRendererState(){return this.state}yieldToMain(){let e=globalThis.scheduler;return e&&typeof e.yield==`function`?e.yield():new Promise(e=>setTimeout(e,0))}renderAtTime(e){let t=this.findCurrentIndex(e);return t<0?void 0:this.renderAtIndex(t)}findCurrentIndex(e){return this.state.useWorker&&this.state.workerReady?A(this.state.timestamps,e*1e3):this.pgsParser?.findIndexAtTimestamp(e)??-1}renderAtIndex(e){if(this.state.frameCache.has(e))return this.state.frameCache.get(e)??void 0;if(this.state.useWorker&&this.state.workerReady){if(!this.state.pendingRenders.has(e)){let t=S({type:`renderPgsAtIndex`,sessionId:this.state.sessionId,index:e}).then(e=>e.type===`pgsFrame`?{frame:e.frame?j(e.frame):null,renderIssue:e.renderIssue?.trim()||null}:{frame:null,renderIssue:null}),n=t.then(({frame:e})=>e);this.state.pendingRenders.set(e,n),this.emitCacheChange(this.state.frameCache.size,this.state.pendingRenders.size),t.then(({frame:t,renderIssue:n})=>{P(this.state,e,t,n),this.state.pendingRenders.delete(e),this.emitCacheChange(this.state.frameCache.size,this.state.pendingRenders.size),this.findCurrentIndex(this.video.currentTime+this.timeOffset)===e&&(this.lastRenderedIndex=-1)})}return}let t=this.pgsParser?.renderAtIndex(e)??null;return P(this.state,e,t,this.pgsParser?.getLastRenderIssue()??null),this.emitCacheChange(this.state.frameCache.size,this.state.pendingRenders.size),t??void 0}buildCueMetadata(e){if(this.pgsParser)return this.pgsParser.getCueMetadata(e);let t=this.state.metadata;if(!t||e<0||e>=this.state.timestamps.length)return null;let n=this.state.timestamps[e],r=this.state.timestamps[e+1]??n+5e3,i=this.state.frameCache.get(e)??null;return{index:e,format:`pgs`,startTime:n,endTime:r,duration:Math.max(0,r-n),screenWidth:t.screenWidth,screenHeight:t.screenHeight,bounds:i?N(i):null,compositionCount:i?.compositionData.length??0}}isPendingRender(e){return this.state.pendingRenders.has(e)}onSeek(){this.state.frameCache.clear(),this.state.renderIssues.clear(),this.state.pendingRenders.clear(),this.emitCacheChange(this.state.frameCache.size,this.state.pendingRenders.size),this.state.useWorker&&this.state.workerReady&&S({type:`clearPgsCache`,sessionId:this.state.sessionId}).catch(()=>{}),this.pgsParser?.clearCache()}setCacheLimit(e){this.cacheLimit=F(this.state,e),this.emitCacheChange(this.state.frameCache.size,this.state.pendingRenders.size)}clearFrameCache(){this.state.frameCache.clear(),this.state.renderIssues.clear(),this.state.pendingRenders.clear(),this.lastRenderedIndex=-1,this.state.useWorker&&this.state.workerReady&&S({type:`clearPgsCache`,sessionId:this.state.sessionId}).catch(()=>{}),this.pgsParser?.clearCache(),this.emitCacheChange(this.state.frameCache.size,this.state.pendingRenders.size)}async prefetchRange(e,t){let n=Math.max(0,Math.min(e,t)),r=Math.min(Math.max(e,t),this.state.timestamps.length-1);for(let e=n;e<=r;e++)this.state.frameCache.has(e)||this.renderAtIndex(e)===void 0&&this.state.pendingRenders.has(e)&&await this.state.pendingRenders.get(e)}async prefetchAroundTime(e,t=this.prefetchBefore,n=this.prefetchAfter){let r=this.findCurrentIndex(e);r<0||await this.prefetchRange(r-t,r+n)}getStats(){return{...this.getBaseStats(),usingWorker:this.state.useWorker&&this.state.workerReady,cachedFrames:this.state.frameCache.size,pendingRenders:this.state.pendingRenders.size,totalEntries:this.state.timestamps.length||(this.pgsParser?.getTimestamps().length??0)}}dispose(){super.dispose(),this.state.frameCache.clear(),this.state.renderIssues.clear(),this.state.pendingRenders.clear(),this.state.useWorker&&this.state.workerReady&&S({type:`disposePgs`,sessionId:this.state.sessionId}).catch(()=>{}),this.pgsParser?.dispose(),this.pgsParser=null,this.state.sessionId=null}};export{ae as PgsRenderer};