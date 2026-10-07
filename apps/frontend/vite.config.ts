import {defineConfig,loadEnv} from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig(({mode})=>{
 const apiTarget=loadEnv(mode,process.cwd(),'').API_PROXY_TARGET||'http://127.0.0.1:8080'
 return {plugins:[vue()],server:{host:'127.0.0.1',proxy:{'/api':{
  target:apiTarget,
  changeOrigin:false
 }}}}
})
