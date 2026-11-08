/**
 * Gate Admin Console - Visual Gateway (Magpie Architecture)
 * 纯轻量交互式 SVG 拓扑流向图、品牌矢量微标、SWR 极速缓存、IP/属地/Key 三维关联与时间滑动条流水
 */

let cachedTrafficData = null;
let trafficSelectedIP = '';
let trafficSelectedKey = '';
let trafficSelectedModel = '';
let trafficSelectedStatus = 'all';
let trafficSearchQuery = '';
let trafficTimer = null;
let trafficPollingInterval = 3;
let trafficHighlightedNode = null;

// 时间范围滑动条状态
let trafficTimeMinPercent = 0;
let trafficTimeMaxPercent = 100;
let trafficBufferMinTs = 0;
let trafficBufferMaxTs = 0;
let trafficActiveTimePreset = 'all';

const TRAFFIC_SWR_CACHE_KEY = 'cpa_traffic_snapshot_cache';

// 1. 矢量微标库 (严格 Lucide 规范，1.8px 细线条，零原生 Emoji)
const TRAFFIC_ICONS = {
  globe: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg>',
  activity: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>',
  zap: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>',
  shield: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>',
  refresh: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M3 21v-5h5"/></svg>',
  layers: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/></svg>',
  search: '<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>',
  mapPin: '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/></svg>',
  server: '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/></svg>',
  arrowRight: '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>'
};

// 2. 品牌几何微标生成器 (SVG 内嵌嵌入)
// 渠道品牌图标：直接复用 magpie (usemagpie.ai) 站点公开图标库的真实品牌 SVG。
// 上游来源：/www/workspace/Project/magpie/site/public/icons/
// 全部资产 viewBox="0 0 24 24"，单 path 为主，可原样内联进节点 <svg>。
// 按 AGENTS.md 开源引用铁律，须在 README 与架构文档标注该模块图标来源归属 magpie。
const BRAND_ICON_PATHS = {
  claude: '<path d="M4.709 15.955l4.72-2.647.08-.23-.08-.128H9.2l-.79-.048-2.698-.073-2.339-.097-2.266-.122-.571-.121L0 11.784l.055-.352.48-.321.686.06 1.52.103 2.278.158 1.652.097 2.449.255h.389l.055-.157-.134-.098-.103-.097-2.358-1.596-2.552-1.688-1.336-.972-.724-.491-.364-.462-.158-1.008.656-.722.881.06.225.061.893.686 1.908 1.476 2.491 1.833.365.304.145-.103.019-.073-.164-.274-1.355-2.446-1.446-2.49-.644-1.032-.17-.619a2.97 2.97 0 01-.104-.729L6.283.134 6.696 0l.996.134.42.364.62 1.414 1.002 2.229 1.555 3.03.456.898.243.832.091.255h.158V9.01l.128-1.706.237-2.095.23-2.695.08-.76.376-.91.747-.492.584.28.48.685-.067.444-.286 1.851-.559 2.903-.364 1.942h.212l.243-.242.985-1.306 1.652-2.064.73-.82.85-.904.547-.431h1.033l.76 1.129-.34 1.166-1.064 1.347-.881 1.142-1.264 1.7-.79 1.36.073.11.188-.02 2.856-.606 1.543-.28 1.841-.315.833.388.091.395-.328.807-1.969.486-2.309.462-3.439.813-.042.03.049.061 1.549.146.662.036h1.622l3.02.225.79.522.474.638-.079.485-1.215.62-1.64-.389-3.829-.91-1.312-.329h-.182v.11l1.093 1.068 2.006 1.81 2.509 2.33.127.578-.322.455-.34-.049-2.205-1.657-.851-.747-1.926-1.62h-.128v.17l.444.649 2.345 3.521.122 1.08-.17.353-.608.213-.668-.122-1.374-1.925-1.415-2.167-1.143-1.943-.14.08-.674 7.254-.316.37-.729.28-.607-.461-.322-.747.322-1.476.389-1.924.315-1.53.286-1.9.17-.632-.012-.042-.14.018-1.434 1.967-2.18 2.945-1.726 1.845-.414.164-.717-.37.067-.662.401-.589 2.388-3.036 1.44-1.882.93-1.086-.006-.158h-.055L4.132 18.56l-1.13.146-.487-.456.061-.746.231-.243 1.908-1.312-.006.006z" fill="#D97757" fill-rule="nonzero"></path>',
  gemini: '<path d="M20.616 10.835a14.147 14.147 0 01-4.45-3.001 14.111 14.111 0 01-3.678-6.452.503.503 0 00-.975 0 14.134 14.134 0 01-3.679 6.452 14.155 14.155 0 01-4.45 3.001c-.65.28-1.318.505-2.002.678a.502.502 0 000 .975c.684.172 1.35.397 2.002.677a14.147 14.147 0 014.45 3.001 14.112 14.112 0 013.679 6.453.502.502 0 00.975 0c.172-.685.397-1.351.677-2.003a14.145 14.145 0 013.001-4.45 14.113 14.113 0 016.453-3.678.503.503 0 000-.975 13.245 13.245 0 01-2.003-.678z" fill="#3186FF"></path><path d="M20.616 10.835a14.147 14.147 0 01-4.45-3.001 14.111 14.111 0 01-3.678-6.452.503.503 0 00-.975 0 14.134 14.134 0 01-3.679 6.452 14.155 14.155 0 01-4.45 3.001c-.65.28-1.318.505-2.002.678a.502.502 0 000 .975c.684.172 1.35.397 2.002.677a14.147 14.147 0 014.45 3.001 14.112 14.112 0 013.679 6.453.502.502 0 00.975 0c.172-.685.397-1.351.677-2.003a14.145 14.145 0 013.001-4.45 14.113 14.113 0 016.453-3.678.503.503 0 000-.975 13.245 13.245 0 01-2.003-.678z" fill="url(#lobe-icons-gemini-0-_R_0_)"></path><path d="M20.616 10.835a14.147 14.147 0 01-4.45-3.001 14.111 14.111 0 01-3.678-6.452.503.503 0 00-.975 0 14.134 14.134 0 01-3.679 6.452 14.155 14.155 0 01-4.45 3.001c-.65.28-1.318.505-2.002.678a.502.502 0 000 .975c.684.172 1.35.397 2.002.677a14.147 14.147 0 014.45 3.001 14.112 14.112 0 013.679 6.453.502.502 0 00.975 0c.172-.685.397-1.351.677-2.003a14.145 14.145 0 013.001-4.45 14.113 14.113 0 016.453-3.678.503.503 0 000-.975 13.245 13.245 0 01-2.003-.678z" fill="url(#lobe-icons-gemini-1-_R_0_)"></path><path d="M20.616 10.835a14.147 14.147 0 01-4.45-3.001 14.111 14.111 0 01-3.678-6.452.503.503 0 00-.975 0 14.134 14.134 0 01-3.679 6.452 14.155 14.155 0 01-4.45 3.001c-.65.28-1.318.505-2.002.678a.502.502 0 000 .975c.684.172 1.35.397 2.002.677a14.147 14.147 0 014.45 3.001 14.112 14.112 0 013.679 6.453.502.502 0 00.975 0c.172-.685.397-1.351.677-2.003a14.145 14.145 0 013.001-4.45 14.113 14.113 0 016.453-3.678.503.503 0 000-.975 13.245 13.245 0 01-2.003-.678z" fill="url(#lobe-icons-gemini-2-_R_0_)"></path><defs><linearGradient gradientUnits="userSpaceOnUse" id="lobe-icons-gemini-0-_R_0_" x1="7" x2="11" y1="15.5" y2="12"><stop stop-color="#08B962"></stop><stop offset="1" stop-color="#08B962" stop-opacity="0"></stop></linearGradient><linearGradient gradientUnits="userSpaceOnUse" id="lobe-icons-gemini-1-_R_0_" x1="8" x2="11.5" y1="5.5" y2="11"><stop stop-color="#F94543"></stop><stop offset="1" stop-color="#F94543" stop-opacity="0"></stop></linearGradient><linearGradient gradientUnits="userSpaceOnUse" id="lobe-icons-gemini-2-_R_0_" x1="3.5" x2="17.5" y1="13.5" y2="12"><stop stop-color="#FABC12"></stop><stop offset=".46" stop-color="#FABC12" stop-opacity="0"></stop></linearGradient></defs>',
  deepseek: '<path d="M23.748 4.482c-.254-.124-.364.113-.512.234-.051.039-.094.09-.137.136-.372.397-.806.657-1.373.626-.829-.046-1.537.214-2.163.848-.133-.782-.575-1.248-1.247-1.548-.352-.156-.708-.311-.955-.65-.172-.241-.219-.51-.305-.774-.055-.16-.11-.323-.293-.35-.2-.031-.278.136-.356.276-.313.572-.434 1.202-.422 1.84.027 1.436.633 2.58 1.838 3.393.137.093.172.187.129.323-.082.28-.18.552-.266.833-.055.179-.137.217-.329.14a5.526 5.526 0 01-1.736-1.18c-.857-.828-1.631-1.742-2.597-2.458a11.365 11.365 0 00-.689-.471c-.985-.957.13-1.743.388-1.836.27-.098.093-.432-.779-.428-.872.004-1.67.295-2.687.684a3.055 3.055 0 01-.465.137 9.597 9.597 0 00-2.883-.102c-1.885.21-3.39 1.102-4.497 2.623C.082 8.606-.231 10.684.152 12.85c.403 2.284 1.569 4.175 3.36 5.653 1.858 1.533 3.997 2.284 6.438 2.14 1.482-.085 3.133-.284 4.994-1.86.47.234.962.327 1.78.397.63.059 1.236-.03 1.705-.128.735-.156.684-.837.419-.961-2.155-1.004-1.682-.595-2.113-.926 1.096-1.296 2.746-2.642 3.392-7.003.05-.347.007-.565 0-.845-.004-.17.035-.237.23-.256a4.173 4.173 0 001.545-.475c1.396-.763 1.96-2.015 2.093-3.517.02-.23-.004-.467-.247-.588zM11.581 18c-2.089-1.642-3.102-2.183-3.52-2.16-.392.024-.321.471-.235.763.09.288.207.486.371.739.114.167.192.416-.113.603-.673.416-1.842-.14-1.897-.167-1.361-.802-2.5-1.86-3.301-3.307-.774-1.393-1.224-2.887-1.298-4.482-.02-.386.093-.522.477-.592a4.696 4.696 0 011.529-.039c2.132.312 3.946 1.265 5.468 2.774.868.86 1.525 1.887 2.202 2.891.72 1.066 1.494 2.082 2.48 2.914.348.292.625.514.891.677-.802.09-2.14.11-3.054-.614zm1-6.44a.306.306 0 01.415-.287.302.302 0 01.2.288.306.306 0 01-.31.307.303.303 0 01-.304-.308zm3.11 1.596c-.2.081-.399.151-.59.16a1.245 1.245 0 01-.798-.254c-.274-.23-.47-.358-.552-.758a1.73 1.73 0 01.016-.588c.07-.327-.008-.537-.239-.727-.187-.156-.426-.199-.688-.199a.559.559 0 01-.254-.078c-.11-.054-.2-.19-.114-.358.028-.054.16-.186.192-.21.356-.202.767-.136 1.146.016.352.144.618.408 1.001.782.391.451.462.576.685.914.176.265.336.537.445.848.067.195-.019.354-.25.452z" fill="#4D6BFE"></path>',
  openai: '<path d="M9.205 8.658v-2.26c0-.19.072-.333.238-.428l4.543-2.616c.619-.357 1.356-.523 2.117-.523 2.854 0 4.662 2.212 4.662 4.566 0 .167 0 .357-.024.547l-4.71-2.759a.797.797 0 00-.856 0l-5.97 3.473zm10.609 8.8V12.06c0-.333-.143-.57-.429-.737l-5.97-3.473 1.95-1.118a.433.433 0 01.476 0l4.543 2.617c1.309.76 2.189 2.378 2.189 3.948 0 1.808-1.07 3.473-2.76 4.163zM7.802 12.703l-1.95-1.142c-.167-.095-.239-.238-.239-.428V5.899c0-2.545 1.95-4.472 4.591-4.472 1 0 1.927.333 2.712.928L8.23 5.067c-.285.166-.428.404-.428.737v6.898zM12 15.128l-2.795-1.57v-3.33L12 8.658l2.795 1.57v3.33L12 15.128zm1.796 7.23c-1 0-1.927-.332-2.712-.927l4.686-2.712c.285-.166.428-.404.428-.737v-6.898l1.974 1.142c.167.095.238.238.238.428v5.233c0 2.545-1.974 4.472-4.614 4.472zm-5.637-5.303l-4.544-2.617c-1.308-.761-2.188-2.378-2.188-3.948A4.482 4.482 0 014.21 6.327v5.423c0 .333.143.571.428.738l5.947 3.449-1.95 1.118a.432.432 0 01-.476 0zm-.262 3.9c-2.688 0-4.662-2.021-4.662-4.519 0-.19.024-.38.047-.57l4.686 2.71c.286.167.571.167.856 0l5.97-3.448v2.26c0 .19-.07.333-.237.428l-4.543 2.616c-.619.357-1.356.523-2.117.523zm5.899 2.83a5.947 5.947 0 005.827-4.756C22.287 18.339 24 15.84 24 13.296c0-1.665-.713-3.282-1.998-4.448.119-.5.19-.999.19-1.498 0-3.401-2.759-5.947-5.946-5.947-.642 0-1.26.095-1.88.31A5.962 5.962 0 0010.205 0a5.947 5.947 0 00-5.827 4.757C1.713 5.447 0 7.945 0 10.49c0 1.666.713 3.283 1.998 4.448-.119.5-.19 1-.19 1.499 0 3.401 2.759 5.946 5.946 5.946.642 0 1.26-.095 1.88-.309a5.96 5.96 0 004.162 1.713z"></path>',
  xai: '<path d="M6.469 8.776L16.512 23h-4.464L2.005 8.776H6.47zm-.004 7.9l2.233 3.164L6.467 23H2l4.465-6.324zM22 2.582V23h-3.659V7.764L22 2.582zM22 1l-9.952 14.095-2.233-3.163L17.533 1H22z"></path>',
  kimi: '<path d="M21.846 0a1.923 1.923 0 110 3.846H20.15a.226.226 0 01-.227-.226V1.923C19.923.861 20.784 0 21.846 0z"></path><path d="M11.065 11.199l7.257-7.2c.137-.136.06-.41-.116-.41H14.3a.164.164 0 00-.117.051l-7.82 7.756c-.122.12-.302.013-.302-.179V3.82c0-.127-.083-.23-.185-.23H3.186c-.103 0-.186.103-.186.23V19.77c0 .128.083.23.186.23h2.69c.103 0 .186-.102.186-.23v-3.25c0-.069.025-.135.069-.178l2.424-2.406a.158.158 0 01.205-.023l6.484 4.772a7.677 7.677 0 003.453 1.283c.108.012.2-.095.2-.23v-3.06c0-.117-.07-.212-.164-.227a5.028 5.028 0 01-2.027-.807l-5.613-4.064c-.117-.078-.132-.279-.028-.381z"></path>',
  minimax: '<defs><linearGradient id="lobe-icons-minimax-_R_0_" x1="0%" x2="100.182%" y1="50.057%" y2="50.057%"><stop offset="0%" stop-color="#E2167E"></stop><stop offset="100%" stop-color="#FE603C"></stop></linearGradient></defs><path d="M16.278 2c1.156 0 2.093.927 2.093 2.07v12.501a.74.74 0 00.744.709.74.74 0 00.743-.709V9.099a2.06 2.06 0 012.071-2.049A2.06 2.06 0 0124 9.1v6.561a.649.649 0 01-.652.645.649.649 0 01-.653-.645V9.1a.762.762 0 00-.766-.758.762.762 0 00-.766.758v7.472a2.037 2.037 0 01-2.048 2.026 2.037 2.037 0 01-2.048-2.026v-12.5a.785.785 0 00-.788-.753.785.785 0 00-.789.752l-.001 15.904A2.037 2.037 0 0113.441 22a2.037 2.037 0 01-2.048-2.026V18.04c0-.356.292-.645.652-.645.36 0 .652.289.652.645v1.934c0 .263.142.506.372.638.23.131.514.131.744 0a.734.734 0 00.372-.638V4.07c0-1.143.937-2.07 2.093-2.07zm-5.674 0c1.156 0 2.093.927 2.093 2.07v11.523a.648.648 0 01-.652.645.648.648 0 01-.652-.645V4.07a.785.785 0 00-.789-.78.785.785 0 00-.789.78v14.013a2.06 2.06 0 01-2.07 2.048 2.06 2.06 0 01-2.071-2.048V9.1a.762.762 0 00-.766-.758.762.762 0 00-.766.758v3.8a2.06 2.06 0 01-2.071 2.049A2.06 2.06 0 010 12.9v-1.378c0-.357.292-.646.652-.646.36 0 .653.29.653.646V12.9c0 .418.343.757.766.757s.766-.339.766-.757V9.099a2.06 2.06 0 012.07-2.048 2.06 2.06 0 012.071 2.048v8.984c0 .419.343.758.767.758.423 0 .766-.339.766-.758V4.07c0-1.143.937-2.07 2.093-2.07z" fill="url(#lobe-icons-minimax-_R_0_)" fill-rule="nonzero"></path>',
  qwen: '<path d="M12.604 1.34c.393.69.784 1.382 1.174 2.075a.18.18 0 00.157.091h5.552c.174 0 .322.11.446.327l1.454 2.57c.19.337.24.478.024.837-.26.43-.513.864-.76 1.3l-.367.658c-.106.196-.223.28-.04.512l2.652 4.637c.172.301.111.494-.043.77-.437.785-.882 1.564-1.335 2.34-.159.272-.352.375-.68.37-.777-.016-1.552-.01-2.327.016a.099.099 0 00-.081.05 575.097 575.097 0 01-2.705 4.74c-.169.293-.38.363-.725.364-.997.003-2.002.004-3.017.002a.537.537 0 01-.465-.271l-1.335-2.323a.09.09 0 00-.083-.049H4.982c-.285.03-.553-.001-.805-.092l-1.603-2.77a.543.543 0 01-.002-.54l1.207-2.12a.198.198 0 000-.197 550.951 550.951 0 01-1.875-3.272l-.79-1.395c-.16-.31-.173-.496.095-.965.465-.813.927-1.625 1.387-2.436.132-.234.304-.334.584-.335a338.3 338.3 0 012.589-.001.124.124 0 00.107-.063l2.806-4.895a.488.488 0 01.422-.246c.524-.001 1.053 0 1.583-.006L11.704 1c.341-.003.724.032.9.34zm-3.432.403a.06.06 0 00-.052.03L6.254 6.788a.157.157 0 01-.135.078H3.253c-.056 0-.07.025-.041.074l5.81 10.156c.025.042.013.062-.034.063l-2.795.015a.218.218 0 00-.2.116l-1.32 2.31c-.044.078-.021.118.068.118l5.716.008c.046 0 .08.02.104.061l1.403 2.454c.046.081.092.082.139 0l5.006-8.76.783-1.382a.055.055 0 01.096 0l1.424 2.53a.122.122 0 00.107.062l2.763-.02a.04.04 0 00.035-.02.041.041 0 000-.04l-2.9-5.086a.108.108 0 010-.113l.293-.507 1.12-1.977c.024-.041.012-.062-.035-.062H9.2c-.059 0-.073-.026-.043-.077l1.434-2.505a.107.107 0 000-.114L9.225 1.774a.06.06 0 00-.053-.031zm6.29 8.02c.046 0 .058.02.034.06l-.832 1.465-2.613 4.585a.056.056 0 01-.05.029.058.058 0 01-.05-.029L8.498 9.841c-.02-.034-.01-.052.028-.054l.216-.012 6.722-.012z" fill="url(#lobe-icons-qwen-_R_0_)" fill-rule="nonzero"></path><defs><linearGradient id="lobe-icons-qwen-_R_0_" x1="0%" x2="100%" y1="0%" y2="0%"><stop offset="0%" stop-color="#6336E7" stop-opacity=".84"></stop><stop offset="100%" stop-color="#6F69F7" stop-opacity=".84"></stop></linearGradient></defs>',
  zhipu: '<path d="M11.991 23.503a.24.24 0 00-.244.248.24.24 0 00.244.249.24.24 0 00.245-.249.24.24 0 00-.22-.247l-.025-.001zM9.671 5.365a1.697 1.697 0 011.099 2.132l-.071.172-.016.04-.018.054c-.07.16-.104.32-.104.498-.035.71.47 1.279 1.186 1.314h.366c1.309.053 2.338 1.173 2.286 2.523-.052 1.332-1.152 2.38-2.478 2.327h-.174c-.715.018-1.274.64-1.239 1.368 0 .124.018.23.053.337.209.373.54.658.96.8.75.23 1.517-.125 1.9-.782l.018-.035c.402-.64 1.17-.96 1.92-.711.854.284 1.378 1.226 1.099 2.167a1.661 1.661 0 01-2.077 1.102 1.711 1.711 0 01-.907-.711l-.017-.035c-.2-.323-.463-.58-.851-.711l-.056-.018a1.646 1.646 0 00-1.954.746 1.66 1.66 0 01-1.065.764 1.677 1.677 0 01-1.989-1.279c-.209-.906.332-1.83 1.257-2.043a1.51 1.51 0 01.296-.035h.018c.68-.071 1.151-.622 1.116-1.333a1.307 1.307 0 00-.227-.693 2.515 2.515 0 01-.366-1.403 2.39 2.39 0 01.366-1.208c.14-.195.21-.444.227-.693.018-.71-.506-1.261-1.186-1.332l-.07-.018a1.43 1.43 0 01-.299-.07l-.05-.019a1.7 1.7 0 01-1.047-2.114 1.68 1.68 0 012.094-1.101zm-5.575 10.11c.26-.264.639-.367.994-.27.355.096.633.379.728.74.095.362-.007.748-.267 1.013-.402.41-1.053.41-1.455 0a1.062 1.062 0 010-1.482zm14.845-.294c.359-.09.738.024.992.297.254.274.344.665.237 1.025-.107.36-.396.634-.756.718-.551.128-1.1-.22-1.23-.781a1.05 1.05 0 01.757-1.26zm-.064-4.39c.314.32.49.753.49 1.206 0 .452-.176.886-.49 1.206-.315.32-.74.5-1.185.5-.444 0-.87-.18-1.184-.5a1.727 1.727 0 010-2.412 1.654 1.654 0 012.369 0zm-11.243.163c.364.484.447 1.128.218 1.691a1.665 1.665 0 01-2.188.923c-.855-.36-1.26-1.358-.907-2.228a1.68 1.68 0 011.33-1.038c.593-.08 1.183.169 1.547.652zm11.545-4.221c.368 0 .708.2.892.524.184.324.184.724 0 1.048a1.026 1.026 0 01-.892.524c-.568 0-1.03-.47-1.03-1.048 0-.579.462-1.048 1.03-1.048zm-14.358 0c.368 0 .707.2.891.524.184.324.184.724 0 1.048a1.026 1.026 0 01-.891.524c-.569 0-1.03-.47-1.03-1.048 0-.579.461-1.048 1.03-1.048zm10.031-1.475c.925 0 1.675.764 1.675 1.706s-.75 1.705-1.675 1.705-1.674-.763-1.674-1.705c0-.942.75-1.706 1.674-1.706zm-2.626-.684c.362-.082.653-.356.761-.718a1.062 1.062 0 00-.238-1.028 1.017 1.017 0 00-.996-.294c-.547.14-.881.7-.752 1.257.13.558.675.907 1.225.783zm0 16.876c.359-.087.644-.36.75-.72a1.062 1.062 0 00-.237-1.019 1.018 1.018 0 00-.985-.301 1.037 1.037 0 00-.762.717c-.108.361-.017.754.239 1.028.245.263.606.377.953.305l.043-.01zM17.19 3.5a.631.631 0 00.628-.64c0-.355-.279-.64-.628-.64a.631.631 0 00-.628.64c0 .355.28.64.628.64zm-10.38 0a.631.631 0 00.628-.64c0-.355-.28-.64-.628-.64a.631.631 0 00-.628.64c0 .355.279.64.628.64zm-5.182 7.852a.631.631 0 00-.628.64c0 .354.28.639.628.639a.63.63 0 00.627-.606l.001-.034a.62.62 0 00-.628-.64zm5.182 9.13a.631.631 0 00-.628.64c0 .355.279.64.628.64a.631.631 0 00.628-.64c0-.355-.28-.64-.628-.64zm10.38.018a.631.631 0 00-.628.64c0 .355.28.64.628.64a.631.631 0 00.628-.64c0-.355-.279-.64-.628-.64zm5.182-9.148a.631.631 0 00-.628.64c0 .354.279.639.628.639a.631.631 0 00.628-.64c0-.355-.28-.64-.628-.64zm-.384-4.992a.24.24 0 00.244-.249.24.24 0 00-.244-.249.24.24 0 00-.244.249c0 .142.122.249.244.249zM11.991.497a.24.24 0 00.245-.248A.24.24 0 0011.99 0a.24.24 0 00-.244.249c0 .133.108.236.223.247l.021.001zM2.011 6.36a.24.24 0 00.245-.249.24.24 0 00-.244-.249.24.24 0 00-.244.249.24.24 0 00.244.249zm0 11.263a.24.24 0 00-.243.248.24.24 0 00.244.249.24.24 0 00.244-.249.252.252 0 00-.244-.248zm19.995-.018a.24.24 0 00-.245.248.24.24 0 00.245.25.24.24 0 00.244-.25.252.252 0 00-.244-.248z" fill="#3859FF" fill-rule="nonzero"></path>',
  antigravity: '<mask height="23" id="lobe-icons-antigravity-0-_R_0_" maskUnits="userSpaceOnUse" width="24" x="0" y="1"><path d="M21.751 22.607c1.34 1.005 3.35.335 1.508-1.508C17.73 15.74 18.904 1 12.037 1 5.17 1 6.342 15.74.815 21.1c-2.01 2.009.167 2.511 1.507 1.506 5.192-3.517 4.857-9.714 9.715-9.714 4.857 0 4.522 6.197 9.714 9.715z" fill="#fff"></path></mask><g mask="url(#lobe-icons-antigravity-0-_R_0_)"><g filter="url(#lobe-icons-antigravity-1-_R_0_)"><path d="M-1.018-3.992c-.408 3.591 2.686 6.89 6.91 7.37 4.225.48 7.98-2.043 8.387-5.633.408-3.59-2.686-6.89-6.91-7.37-4.225-.479-7.98 2.043-8.387 5.633z" fill="#FFE432"></path></g><g filter="url(#lobe-icons-antigravity-2-_R_0_)"><path d="M15.269 7.747c1.058 4.557 5.691 7.374 10.348 6.293 4.657-1.082 7.575-5.653 6.516-10.21-1.058-4.556-5.691-7.374-10.348-6.292-4.657 1.082-7.575 5.653-6.516 10.21z" fill="#FC413D"></path></g><g filter="url(#lobe-icons-antigravity-3-_R_0_)"><path d="M-12.443 10.804c1.338 4.703 7.36 7.11 13.453 5.378 6.092-1.733 9.947-6.95 8.61-11.652C8.282-.173 2.26-2.58-3.833-.848-9.925.884-13.78 6.1-12.443 10.804z" fill="#00B95C"></path></g><g filter="url(#lobe-icons-antigravity-4-_R_0_)"><path d="M-12.443 10.804c1.338 4.703 7.36 7.11 13.453 5.378 6.092-1.733 9.947-6.95 8.61-11.652C8.282-.173 2.26-2.58-3.833-.848-9.925.884-13.78 6.1-12.443 10.804z" fill="#00B95C"></path></g><g filter="url(#lobe-icons-antigravity-5-_R_0_)"><path d="M-7.608 14.703c3.352 3.424 9.126 3.208 12.896-.483 3.77-3.69 4.108-9.459.756-12.883C2.69-2.087-3.083-1.871-6.853 1.82c-3.77 3.69-4.108 9.458-.755 12.883z" fill="#00B95C"></path></g><g filter="url(#lobe-icons-antigravity-6-_R_0_)"><path d="M9.932 27.617c1.04 4.482 5.384 7.303 9.7 6.3 4.316-1.002 6.971-5.448 5.93-9.93-1.04-4.483-5.384-7.304-9.7-6.301-4.316 1.002-6.971 5.448-5.93 9.93z" fill="#3186FF"></path></g><g filter="url(#lobe-icons-antigravity-7-_R_0_)"><path d="M2.572-8.185C.392-3.329 2.778 2.472 7.9 4.771c5.122 2.3 11.042.227 13.222-4.63 2.18-4.855-.205-10.656-5.327-12.955-5.122-2.3-11.042-.227-13.222 4.63z" fill="#FBBC04"></path></g><g filter="url(#lobe-icons-antigravity-8-_R_0_)"><path d="M-3.267 38.686c-5.277-2.072 3.742-19.117 5.984-24.83 2.243-5.712 8.34-8.664 13.616-6.592 5.278 2.071 11.533 13.482 9.29 19.195-2.242 5.713-23.613 14.298-28.89 12.227z" fill="#3186FF"></path></g><g filter="url(#lobe-icons-antigravity-9-_R_0_)"><path d="M28.71 17.471c-1.413 1.649-5.1.808-8.236-1.878-3.135-2.687-4.531-6.201-3.118-7.85 1.412-1.649 5.1-.808 8.235 1.878s4.532 6.2 3.119 7.85z" fill="#749BFF"></path></g><g filter="url(#lobe-icons-antigravity-10-_R_0_)"><path d="M18.163 9.077c5.81 3.93 12.502 4.19 14.946.577 2.443-3.612-.287-9.727-6.098-13.658-5.81-3.931-12.502-4.19-14.946-.577-2.443 3.612.287 9.727 6.098 13.658z" fill="#FC413D"></path></g><g filter="url(#lobe-icons-antigravity-11-_R_0_)"><path d="M-.915 2.684c-1.44 3.473-.97 6.967 1.05 7.804 2.02.837 4.824-1.3 6.264-4.772 1.44-3.473.97-6.967-1.05-7.804-2.02-.837-4.824 1.3-6.264 4.772z" fill="#FFEE48"></path></g></g><defs><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="17.587" id="lobe-icons-antigravity-1-_R_0_" width="19.838" x="-3.288" y="-11.917"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="1.117"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="38.565" id="lobe-icons-antigravity-2-_R_0_" width="38.9" x="4.251" y="-13.493"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="5.4"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="36.517" id="lobe-icons-antigravity-3-_R_0_" width="40.955" x="-21.889" y="-10.592"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="4.591"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="36.517" id="lobe-icons-antigravity-4-_R_0_" width="40.955" x="-21.889" y="-10.592"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="4.591"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="36.595" id="lobe-icons-antigravity-5-_R_0_" width="36.632" x="-19.099" y="-10.278"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="4.591"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="34.087" id="lobe-icons-antigravity-6-_R_0_" width="33.533" x=".981" y="8.758"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="4.363"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="35.276" id="lobe-icons-antigravity-7-_R_0_" width="35.978" x="-6.143" y="-21.659"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="3.954"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="46.523" id="lobe-icons-antigravity-8-_R_0_" width="45.114" x="-11.96" y="-.46"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="3.531"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="24.054" id="lobe-icons-antigravity-9-_R_0_" width="25.094" x="10.485" y=".58"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="3.159"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="30.007" id="lobe-icons-antigravity-10-_R_0_" width="33.508" x="5.833" y="-12.467"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="2.669"></feGaussianBlur></filter><filter color-interpolation-filters="sRGB" filterUnits="userSpaceOnUse" height="26.151" id="lobe-icons-antigravity-11-_R_0_" width="22.194" x="-8.355" y="-8.876"><feFlood flood-opacity="0" result="BackgroundImageFix"></feFlood><feBlend in="SourceGraphic" in2="BackgroundImageFix" result="shape"></feBlend><feGaussianBlur result="effect1_foregroundBlur_977_115" stdDeviation="3.303"></feGaussianBlur></filter></defs>',
  opencode: '<path d="M16 6H8v12h8V6zm4 16H4V2h16v20z"></path>',
  openrouter: '<path d="M18.654 3.87a5.087 5.087 0 110 10.174L23.7 19.09c.64.641.187 1.737-.72 1.737H8.48a8.479 8.479 0 010-16.958h10.175zM8.479 7.26a5.087 5.087 0 100 10.176 5.087 5.087 0 000-10.175z"></path>',
  mistral: '<path d="M3.428 3.4h3.429v3.428H3.428V3.4zm13.714 0h3.43v3.428h-3.43V3.4z" fill="gold"></path><path d="M3.428 6.828h6.857v3.429H3.429V6.828zm10.286 0h6.857v3.429h-6.857V6.828z" fill="#FFAF00"></path><path d="M3.428 10.258h17.144v3.428H3.428v-3.428z" fill="#FF8205"></path><path d="M3.428 13.686h3.429v3.428H3.428v-3.428zm6.858 0h3.429v3.428h-3.429v-3.428zm6.856 0h3.43v3.428h-3.43v-3.428z" fill="#FA500F"></path><path d="M0 17.114h10.286v3.429H0v-3.429zm13.714 0H24v3.429H13.714v-3.429z" fill="#E10500"></path>',
};

function getBrandNodeVisual(type, rawLabel) {
  const l = (rawLabel || '').toLowerCase();

  // A. Client IP 节点
  if (type === 'client_ip') {
    if (l.startsWith('172.18.') || l.startsWith('172.17.') || l === '172.18.0.1') {
      return {
        icon: '<rect x="2" y="3.5" width="12" height="9" rx="2" fill="none" stroke="#0284c7" stroke-width="1.4"/><path d="M5 3.5V2M8 3.5V2M11 3.5V2" stroke="#0284c7" stroke-width="1.4"/>',
        borderColor: 'rgba(2, 132, 199, 0.55)',
        bgColor: 'rgba(2, 132, 199, 0.08)',
        badge: 'Bridge',
        color: '#0284c7'
      };
    }
    if (l.startsWith('100.')) {
      return {
        icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#8b5cf6" stroke-width="1.4"/><circle cx="8" cy="8" r="2.2" fill="#8b5cf6"/>',
        borderColor: 'rgba(139, 92, 246, 0.55)',
        bgColor: 'rgba(139, 92, 246, 0.08)',
        badge: 'Tailnet',
        color: '#8b5cf6'
      };
    }
    if (l === '127.0.0.1' || l === 'localhost' || l.startsWith('192.168.') || l.startsWith('10.')) {
      return {
        icon: '<path d="M2 13h12M4 13V6l4-3 4 3v7" fill="none" stroke="#10b981" stroke-width="1.3"/>',
        borderColor: 'rgba(16, 185, 129, 0.55)',
        bgColor: 'rgba(16, 185, 129, 0.08)',
        badge: 'LAN',
        color: '#10b981'
      };
    }
    return {
      icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#06b6d4" stroke-width="1.4"/><path d="M8 2a8.5 8.5 0 0 0 0 12 8.5 8.5 0 0 0 0-12M2 8h12" stroke="#06b6d4" stroke-width="1.2"/>',
      borderColor: 'rgba(6, 182, 212, 0.55)',
      bgColor: 'rgba(6, 182, 212, 0.08)',
      badge: 'WAN',
      color: '#06b6d4'
    };
  }

  // B. 请求模型节点
  if (type === 'model') {
    if (l.includes('claude')) {
      return {
        icon: '<path d="M8 2l1.6 4.4L14 8l-4.4 1.6L8 14l-1.6-4.4L2 8l4.4-1.6z" fill="#d97706" stroke="#b45309" stroke-width="1.2"/>',
        borderColor: 'rgba(217, 119, 6, 0.55)',
        bgColor: 'rgba(217, 119, 6, 0.09)',
        color: '#d97706'
      };
    }
    if (l.includes('gemini') || l.includes('argon')) {
      return {
        icon: '<path d="M8 2c0 3.3-2.7 6-6 6 3.3 0 6 2.7 6 6 0-3.3 2.7-6 6-6-3.3 0-6-2.7-6-6z" fill="#2563eb" stroke="#1d4ed8" stroke-width="1.2"/>',
        borderColor: 'rgba(37, 99, 235, 0.55)',
        bgColor: 'rgba(37, 99, 235, 0.09)',
        color: '#2563eb'
      };
    }
    if (l.includes('step') || l.includes('stepfun')) {
      return {
        icon: '<path fill-rule="evenodd" d="M8 0C3.58 0 0 3.58 0 8s3.58 8 8 8 8-3.58 8-8S12.42 0 8 0ZM5.84 13.07H2.92v-2.92h2.92v2.92ZM9.46 13.07H6.54v-2.92h2.92v2.92ZM9.46 9.46H6.54V6.54h2.92v2.92ZM9.46 5.85H6.54V2.93h2.92v2.92ZM13.07 5.85h-2.92V2.93h2.92v2.92Z" fill="#8b5cf6"/>',
        borderColor: 'rgba(139, 92, 246, 0.55)',
        bgColor: 'rgba(139, 92, 246, 0.09)',
        color: '#8b5cf6'
      };
    }
    if (l.includes('minimax') || l.includes('abab')) {
      return {
        icon: '<path d="M2 7v1a1 1 0 0 0 2 0V5.3a1 1 0 0 1 2 0v5.4a1 1 0 0 0 2 0V2.3a1 1 0 0 1 2 0V9M8 9.3v2.4a1 1 0 0 0 2 0V2.3a1 1 0 0 1 2 0v5.4a1 1 0 0 0 2 0v-2.7a1 1 0 0 0-2 0" fill="none" stroke="#0891b2" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>',
        borderColor: 'rgba(8, 145, 178, 0.55)',
        bgColor: 'rgba(8, 145, 178, 0.09)',
        color: '#0891b2'
      };
    }
    if (l.includes('deepseek')) {
      return {
        icon: '<path d="M3 10.5c3-4 7-5 10.5-4-2 4-5 7-9.5 6-1 0-1-.5-1-2z" fill="#0284c7" stroke="#0369a1" stroke-width="1.2"/>',
        borderColor: 'rgba(2, 132, 199, 0.55)',
        bgColor: 'rgba(2, 132, 199, 0.09)',
        color: '#0284c7'
      };
    }
    if (l.includes('gpt') || l.includes('o1') || l.includes('o3') || l.includes('openai')) {
      return {
        icon: '<path d="M8 2a6 6 0 0 1 5.2 3A6 6 0 0 1 14 11a6 6 0 0 1-5.2 3A6 6 0 0 1 2 11a6 6 0 0 1 6-9z" fill="none" stroke="#10b981" stroke-width="1.6"/>',
        borderColor: 'rgba(16, 185, 129, 0.55)',
        bgColor: 'rgba(16, 185, 129, 0.09)',
        color: '#10b981'
      };
    }
    if (l.includes('qwen') || l.includes('qoder') || l.includes('dfmodel')) {
      return {
        icon: '<polygon points="8 2 14 8 8 14 2 8" fill="none" stroke="#6366f1" stroke-width="1.5"/><circle cx="8" cy="8" r="2" fill="#6366f1"/>',
        borderColor: 'rgba(99, 102, 241, 0.55)',
        bgColor: 'rgba(99, 102, 241, 0.09)',
        color: '#6366f1'
      };
    }
    if (l.includes('probe') || l.includes('unspecified') || l.includes('unknown')) {
      return {
        icon: '<path d="M8 2l5 2.5v4c0 3.5-2.5 5.5-5 6.5-2.5-1-5-3-5-6.5v-4z" fill="none" stroke="#f59e0b" stroke-width="1.4"/><line x1="8" y1="5.5" x2="8" y2="8.5" stroke="#f59e0b" stroke-width="1.5"/><circle cx="8" cy="11" r="0.8" fill="#f59e0b"/>',
        borderColor: 'rgba(245, 158, 11, 0.55)',
        bgColor: 'rgba(245, 158, 11, 0.09)',
        color: '#f59e0b'
      };
    }
    return {
      icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#8b5cf6" stroke-width="1.4"/><path d="M8 5v6M5 8h6" stroke="#8b5cf6" stroke-width="1.4"/>',
      borderColor: 'rgba(139, 92, 246, 0.55)',
      bgColor: 'rgba(139, 92, 246, 0.09)',
      color: '#8b5cf6'
    };
  }

  // C. 路由渠道节点
  // 渠道节点优先使用 magpie 公开图标库的真实品牌 SVG（13 个渠道已覆盖），
  // 仅 stepfun / workbuddy / qoder 等 magpie 未提供图标的渠道回落到下方自绘几何图形。
  if (type === 'channel') {
    const brand = BRAND_ICON_PATHS[l];
    if (brand) {
      return {
        icon: brand,
        borderColor: 'rgba(148, 163, 184, 0.45)',
        bgColor: 'rgba(148, 163, 184, 0.07)',
        color: '#64748b'
      };
    }
  }
  if (type === 'channel') {
    if (l.includes('antigravity')) {
      return {
        icon: '<polygon points="8 2 14 5.5 14 11.5 8 14 2 11.5 2 5.5" fill="none" stroke="#06b6d4" stroke-width="1.4"/><circle cx="8" cy="8" r="2.2" fill="#06b6d4"/>',
        borderColor: 'rgba(6, 182, 212, 0.55)',
        bgColor: 'rgba(6, 182, 212, 0.09)',
        color: '#06b6d4'
      };
    }
    if (l.includes('workbuddy')) {
      return {
        icon: '<polyline points="4 11 8 8 4 5" stroke="#3b82f6" stroke-width="1.8" stroke-linecap="round"/><line x1="9" y1="11" x2="13" y2="11" stroke="#3b82f6" stroke-width="1.8"/>',
        borderColor: 'rgba(59, 130, 246, 0.55)',
        bgColor: 'rgba(59, 130, 246, 0.09)',
        color: '#3b82f6'
      };
    }
    if (l.includes('stepfun') || l.includes('step')) {
      return {
        icon: '<path fill-rule="evenodd" d="M8 0C3.58 0 0 3.58 0 8s3.58 8 8 8 8-3.58 8-8S12.42 0 8 0ZM5.84 13.07H2.92v-2.92h2.92v2.92ZM9.46 13.07H6.54v-2.92h2.92v2.92ZM9.46 9.46H6.54V6.54h2.92v2.92ZM9.46 5.85H6.54V2.93h2.92v2.92ZM13.07 5.85h-2.92V2.93h2.92v2.92Z" fill="#8b5cf6"/>',
        borderColor: 'rgba(139, 92, 246, 0.55)',
        bgColor: 'rgba(139, 92, 246, 0.09)',
        color: '#8b5cf6'
      };
    }
    if (l.includes('minimax')) {
      return {
        icon: '<path d="M2 7v1a1 1 0 0 0 2 0V5.3a1 1 0 0 1 2 0v5.4a1 1 0 0 0 2 0V2.3a1 1 0 0 1 2 0V9M8 9.3v2.4a1 1 0 0 0 2 0V2.3a1 1 0 0 1 2 0v5.4a1 1 0 0 0 2 0v-2.7a1 1 0 0 0-2 0" fill="none" stroke="#0891b2" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>',
        borderColor: 'rgba(8, 145, 178, 0.55)',
        bgColor: 'rgba(8, 145, 178, 0.09)',
        color: '#0891b2'
      };
    }
    // Channel nodes are labelled by provider/credential name, which does not
    // always carry the upstream brand (workbuddy-cn serves deepseek models,
    // "gemini" and "qoder" are providers in their own right). Without these
    // branches every such node fell through to the neutral placeholder, which
    // is what made a topology full of distinct brands look duplicated.
    if (l.includes('deepseek')) {
      return {
        icon: '<path d="M3 10.5c3-4 7-5 10.5-4-2 4-5 7-9.5 6-1 0-1-.5-1-2z" fill="#0284c7" stroke="#0369a1" stroke-width="1.2"/>',
        borderColor: 'rgba(2, 132, 199, 0.55)',
        bgColor: 'rgba(2, 132, 199, 0.09)',
        color: '#0284c7'
      };
    }
    if (l.includes('gemini') || l.includes('google') || l.includes('vertex')) {
      return {
        icon: '<path d="M8 2c0 3.3-2.7 6-6 6 3.3 0 6 2.7 6 6 0-3.3 2.7-6 6-6-3.3 0-6-2.7-6-6z" fill="#2563eb" stroke="#1d4ed8" stroke-width="1.2"/>',
        borderColor: 'rgba(37, 99, 235, 0.55)',
        bgColor: 'rgba(37, 99, 235, 0.09)',
        color: '#2563eb'
      };
    }
    if (l.includes('claude') || l.includes('anthropic')) {
      return {
        icon: '<path d="M8 2l1.6 4.4L14 8l-4.4 1.6L8 14l-1.6-4.4L2 8l4.4-1.6z" fill="#d97706" stroke="#b45309" stroke-width="1.2"/>',
        borderColor: 'rgba(217, 119, 6, 0.55)',
        bgColor: 'rgba(217, 119, 6, 0.09)',
        color: '#d97706'
      };
    }
    if (l.includes('qoder')) {
      return {
        icon: '<circle cx="8" cy="8" r="5.5" fill="none" stroke="#ff6a00" stroke-width="1.5"/><path d="M8 5v6M5 8h6" stroke="#ff6a00" stroke-width="1.5" stroke-linecap="round"/>',
        borderColor: 'rgba(255, 106, 0, 0.55)',
        bgColor: 'rgba(255, 106, 0, 0.09)',
        color: '#ff6a00'
      };
    }
    if (l.includes('openrouter')) {
      return {
        icon: '<path d="M2 5h3l3 3h6M2 11h3l3-3h6" fill="none" stroke="#6366f1" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><circle cx="13" cy="4.5" r="1.6" fill="#6366f1"/><circle cx="13" cy="11.5" r="1.6" fill="#6366f1"/>',
        borderColor: 'rgba(99, 102, 241, 0.55)',
        bgColor: 'rgba(99, 102, 241, 0.09)',
        color: '#6366f1'
      };
    }
    if (l.includes('opencode')) {
      return {
        icon: '<rect x="2" y="3" width="12" height="10" rx="2" fill="none" stroke="#09090b" stroke-width="1.5"/><path d="M5 7l2 2-2 2M8.5 11h3" stroke="#09090b" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>',
        borderColor: 'rgba(9, 9, 11, 0.55)',
        bgColor: 'rgba(9, 9, 11, 0.07)',
        color: '#09090b'
      };
    }
    if (l.includes('zcode') || l.includes('glm') || l.includes('bigmodel')) {
      return {
        icon: '<path d="M8 2l5 3v6l-5 3-5-3V5z" fill="none" stroke="#7c3aed" stroke-width="1.5" stroke-linejoin="round"/><circle cx="8" cy="8" r="2" fill="none" stroke="#7c3aed" stroke-width="1.4"/>',
        borderColor: 'rgba(124, 58, 237, 0.55)',
        bgColor: 'rgba(124, 58, 237, 0.09)',
        color: '#7c3aed'
      };
    }
    if (l.includes('kimi') || l.includes('moonshot')) {
      return {
        icon: '<circle cx="8" cy="8" r="5.5" fill="none" stroke="#0ea5e9" stroke-width="1.5"/><path d="M10.5 5.5a3.5 3.5 0 0 0 0 5" fill="none" stroke="#0ea5e9" stroke-width="1.4" stroke-linecap="round"/>',
        borderColor: 'rgba(14, 165, 233, 0.55)',
        bgColor: 'rgba(14, 165, 233, 0.09)',
        color: '#0ea5e9'
      };
    }
    if (l.includes('xai') || l.includes('grok')) {
      return {
        icon: '<path d="M3 3l10 10M13 3L3 13" stroke="#1f2937" stroke-width="1.6" stroke-linecap="round"/><circle cx="8" cy="8" r="6.2" fill="none" stroke="#1f2937" stroke-width="1.2"/>',
        borderColor: 'rgba(31, 41, 55, 0.55)',
        bgColor: 'rgba(31, 41, 55, 0.07)',
        color: '#1f2937'
      };
    }
    if (l.includes('gateway') || l.includes('probe') || l.includes('unspecified')) {
      return {
        icon: '<path d="M2 8h3l2-3 2 6 2-3h5" fill="none" stroke="#f59e0b" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>',
        borderColor: 'rgba(245, 158, 11, 0.55)',
        bgColor: 'rgba(245, 158, 11, 0.09)',
        color: '#f59e0b'
      };
    }
    return {
      icon: '<rect x="2" y="2.5" width="12" height="4.5" rx="1.5" fill="none" stroke="#64748b" stroke-width="1.3"/><rect x="2" y="9" width="12" height="4.5" rx="1.5" fill="none" stroke="#64748b" stroke-width="1.3"/><circle cx="4.5" cy="4.7" r="0.8" fill="#64748b"/><circle cx="4.5" cy="11.2" r="0.8" fill="#64748b"/>',
      borderColor: 'rgba(100, 116, 139, 0.55)',
      bgColor: 'rgba(100, 116, 139, 0.08)',
      color: '#64748b'
    };
  }

  // D. 状态节点
  if (type === 'status') {
    if (l.includes('2xx') || l.includes('success')) {
      return {
        icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#10b981" stroke-width="1.5"/><polyline points="5.5 8 7.5 10 11 6" stroke="#10b981" stroke-width="1.6" fill="none" stroke-linecap="round"/>',
        borderColor: 'rgba(16, 185, 129, 0.55)',
        bgColor: 'rgba(16, 185, 129, 0.1)',
        color: '#10b981'
      };
    }
    if (l.includes('4xx') || l.includes('499')) {
      return {
        icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#f59e0b" stroke-width="1.5"/><line x1="8" y1="5.5" x2="8" y2="8.5" stroke="#f59e0b" stroke-width="1.5"/><circle cx="8" cy="11" r="0.8" fill="#f59e0b"/>',
        borderColor: 'rgba(245, 158, 11, 0.55)',
        bgColor: 'rgba(245, 158, 11, 0.1)',
        color: '#f59e0b'
      };
    }
    return {
      icon: '<circle cx="8" cy="8" r="6" fill="none" stroke="#ef4444" stroke-width="1.5"/><line x1="5.5" y1="5.5" x2="10.5" y2="10.5" stroke="#ef4444" stroke-width="1.5"/><line x1="10.5" y1="5.5" x2="5.5" y2="10.5" stroke="#ef4444" stroke-width="1.5"/>',
      borderColor: 'rgba(239, 68, 68, 0.55)',
      bgColor: 'rgba(239, 68, 68, 0.1)',
      color: '#ef4444'
    };
  }

  return {
    icon: '',
    borderColor: 'rgba(148, 163, 184, 0.4)',
    bgColor: 'rgba(148, 163, 184, 0.08)',
    color: '#64748b'
  };
}

// 3. SWR 数据拉取调度
async function fetchTrafficData(force = false) {
  if (!cachedTrafficData && !force) {
    try {
      const cachedRaw = sessionStorage.getItem(TRAFFIC_SWR_CACHE_KEY);
      if (cachedRaw) {
        const cached = JSON.parse(cachedRaw);
        if (cached && cached.summary) {
          cachedTrafficData = cached;
          renderTrafficUI(cached, false);
        }
      } else {
        renderTrafficSkeleton();
      }
    } catch (e) {
      renderTrafficSkeleton();
    }
  }

  try {
    const res = await fetch('/console/api/traffic', {
      headers: getAuthHeaders(),
      cache: 'no-store'
    });
    if (!res.ok) return;
    const data = await res.json();
    cachedTrafficData = data;

    try {
      sessionStorage.setItem(TRAFFIC_SWR_CACHE_KEY, JSON.stringify(data));
    } catch (e) {}

    const badge = document.getElementById('nav-badge-traffic');
    if (badge && data.summary) {
      const count = Number(data.summary.active_ips || 0);
      badge.textContent = String(count);
      badge.style.display = count > 0 ? 'inline-flex' : 'none';
    }

    renderTrafficUI(data, false);
  } catch (err) {
    console.error('Failed to fetch visual gateway traffic data:', err);
  }
}

// 4. 骨架屏渲染 (1:1 结构几何微光)
function renderTrafficSkeleton() {
  const container = document.getElementById('traffic-topology-canvas-container');
  if (!container) return;

  container.innerHTML = `
    <div class="traffic-topology-skeleton">
      <div class="traffic-skeleton-col">
        <div class="traffic-skeleton-shimmer traffic-skeleton-head"></div>
        <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
        <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
      </div>
      <div class="traffic-skeleton-col">
        <div class="traffic-skeleton-shimmer traffic-skeleton-head"></div>
        <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
        <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
      </div>
      <div class="traffic-skeleton-col">
        <div class="traffic-skeleton-shimmer traffic-skeleton-head"></div>
        <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
        <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
      </div>
      <div class="traffic-skeleton-col">
        <div class="traffic-skeleton-shimmer traffic-skeleton-head"></div>
        <div class="traffic-skeleton-shimmer traffic-skeleton-card"></div>
      </div>
    </div>
  `;
}

// 5. 渲染完整可视化网关面板
function renderTrafficUI(data, animate = false) {
  if (!data) return;
  const summary = data.summary || { active_ips: 0, requests_5m: 0, avg_latency_ms: 0, success_rate: 0, total_recorded: 0 };

  const elIps = document.getElementById('traffic-val-ips');
  const elReqs = document.getElementById('traffic-val-reqs');
  const elLat = document.getElementById('traffic-val-latency');
  const elSucc = document.getElementById('traffic-val-success');
  const elTotal = document.getElementById('traffic-val-total');

  if (elIps) elIps.innerHTML = `${summary.active_ips || 0}<small>个活跃 IP</small>`;
  if (elReqs) elReqs.innerHTML = `${summary.requests_5m || 0}<small>次 / 5m</small>`;
  if (elLat) elLat.innerHTML = `${Math.round(summary.avg_latency_ms || 0)}<small>ms</small>`;
  if (elSucc) elSucc.innerHTML = `${((summary.success_rate || 0) * 100).toFixed(1)}<small>%</small>`;
  if (elTotal) elTotal.textContent = `环形缓冲累计: ${summary.total_recorded || 0} 条`;

  // 增量 Diff 渲染拓扑 (零销毁、零闪烁)
  renderTrafficTopology(data, animate);
  renderTrafficIPLedger(data.ip_stats || []);
  renderTrafficGeoDistribution(data.geo_stats || []);
}

// 6. 增量 DOM Diff 渲染拓扑图 (零销毁、零闪烁、平滑位移)
function renderTrafficTopology(data, animate = false) {
  const container = document.getElementById('traffic-topology-canvas-container');
  if (!container) return;

  const records = data.records || data.recent_requests || [];
  if (records.length === 0) {
    container.innerHTML = `
      <div style="padding:48px 16px;text-align:center;color:var(--text-muted);">
        <div style="display:inline-flex;padding:12px;border-radius:12px;background:rgba(59,130,246,0.06);color:#3b82f6;margin-bottom:12px;">
          ${TRAFFIC_ICONS.layers}
        </div>
        <div style="font-size:14px;font-weight:600;color:var(--text-main);margin-bottom:4px;">暂无实时流量记录</div>
        <div style="font-size:12px;color:var(--text-muted);max-width:420px;margin:0 auto;">网关正在实时侦听多用户调用，产生真实请求后全链路流向图将在此实时呈现。</div>
      </div>
    `;
    return;
  }

  const topology = data.topology || { nodes: [], links: [] };
  const colIPs = (topology.nodes || []).filter(n => n.type === 'client_ip').slice(0, 8);
  const colModels = (topology.nodes || []).filter(n => n.type === 'model').slice(0, 8);
  const colChannels = (topology.nodes || []).filter(n => n.type === 'channel').slice(0, 8);
  const colStatuses = (topology.nodes || []).filter(n => n.type === 'status').slice(0, 8);

  const maxNodes = Math.max(colIPs.length, colModels.length, colChannels.length, colStatuses.length, 1);
  const svgHeight = Math.max(260, maxNodes * 56 + 48);
  const nodeW = 168;
  const nodeH = 38;
  const nodePositions = {};

  // 各品牌 path 的有效图形尺寸差异极大（gateway 波形线实测仅 14x6，deepseek 10.5x6.6，
  // stepfun 铺满 16x16）。统一 viewBox 盒子只能让"容器"等大，里面的图形仍然大小不一。
  // 这里在插入 DOM 前，用一次性离屏 svg 量出原始 bbox，再把每个子元素包一层 transform，
  // 使有效图形占满画布的 84% 并居中。用离屏测量是为了避开 getBBox 的时序陷阱：
  // 已带 transform 的子元素再次调用时会返回变换后的结果，导致二次缩放越缩越小。
  const ICON_NORM_TARGET = 16 * 0.84;
  const measureIconBBox = (iconMarkup) => {
    try {
      const probe = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      probe.setAttribute('viewBox', '0 0 16 16');
      probe.setAttribute('width', '0');
      probe.setAttribute('height', '0');
      probe.style.cssText = 'position:absolute;left:-9999px;top:-9999px;';
      probe.innerHTML = iconMarkup;
      document.body.appendChild(probe);
      const b = probe.getBBox();
      probe.remove();
      if (!b.width || !b.height) return null;
      return { x: b.x, y: b.y, width: b.width, height: b.height };
    } catch (e) { return null; }
  };

  const normalizeNodeIcon = (iconSvg, rawIconMarkup) => {
    if (!iconSvg || iconSvg.tagName !== 'svg') return;
    if (iconSvg.getAttribute('data-icon-normalized') === '1') return;
    const b = measureIconBBox(rawIconMarkup || '');
    if (!b) return;
    // 归一化策略：让图形的短边铺满目标，视觉尺寸就齐平了。
    // 但不能只按短边算——gateway 波形线是 14x6 的宽图形，按短边（6）放大 2.24 倍后
    // 宽会变成 31.4，溢出图标盒并顶到旁边的节点文字。所以长边设 1.55 倍目标的上限：
    // 超限就退回到按长边算，牺牲一点高度换取不入侵文字区。
    const maxLongEdge = ICON_NORM_TARGET * 1.55;
    const scaleByShort = ICON_NORM_TARGET / Math.min(b.width, b.height);
    const scaleByLong = maxLongEdge / Math.max(b.width, b.height);
    const scale = Math.min(scaleByShort, scaleByLong);
    const cx = b.x + b.width / 2;
    const cy = b.y + b.height / 2;
    const dx = 8 - cx * scale;
    const dy = 8 - cy * scale;
    iconSvg.setAttribute('data-icon-normalized', '1');
    iconSvg.querySelectorAll('path, rect, circle, ellipse, line, polyline, polygon').forEach((el) => {
      el.setAttribute('transform', `translate(${dx.toFixed(3)}, ${dy.toFixed(3)}) scale(${scale.toFixed(3)})`);
    });
  };



  const calcPositions = (items, x) => {
    const count = items.length;
    const step = (svgHeight - 48) / Math.max(1, count);
    for (let i = 0; i < count; i++) {
      const item = items[i];
      const yPos = 36 + i * step + (step - nodeH) / 2;
      nodePositions[item.id] = { x, y: yPos, item };
    }
  };

  calcPositions(colIPs, 24);
  calcPositions(colModels, 280);
  calcPositions(colChannels, 536);
  calcPositions(colStatuses, 792);

  // 1. 初始化持久化外层 SVG (仅在初次或被清空时创建)
  let svgEl = document.getElementById('traffic-topology-svg');
  if (!svgEl || !container.contains(svgEl)) {
    container.innerHTML = `
      <svg id="traffic-topology-svg" viewBox="0 0 984 ${svgHeight}" style="width:100%;min-width:940px;height:${svgHeight}px;display:block;">
        <defs>
          <linearGradient id="grad-active-link" x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" stop-color="#06b6d4" />
            <stop offset="100%" stop-color="#3b82f6" />
          </linearGradient>
        </defs>
        <text x="24" y="20" fill="#0891b2" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 1 · CLIENT IP</text>
        <text x="280" y="20" fill="#7c3aed" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 2 · REQUEST MODEL</text>
        <text x="536" y="20" fill="#d97706" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 3 · ROUTED CHANNEL</text>
        <text x="792" y="20" fill="#059669" font-size="11" font-weight="700" letter-spacing="0.04em">STAGE 4 · OUTCOME STATUS</text>
        <g id="traffic-links-layer"></g>
        <g id="traffic-nodes-layer"></g>
      </svg>
    `;
    svgEl = document.getElementById('traffic-topology-svg');
  } else {
    svgEl.setAttribute('viewBox', `0 0 984 ${svgHeight}`);
    svgEl.style.height = `${svgHeight}px`;
  }

  const linksLayer = document.getElementById('traffic-links-layer');
  const nodesLayer = document.getElementById('traffic-nodes-layer');
  if (!linksLayer || !nodesLayer) return;

  // 2. 有向 DAG 高亮拓扑计算
  const isFiltered = Boolean(trafficHighlightedNode);
  const activeLinks = new Set();
  const activeNodes = new Set();

  if (isFiltered) {
    activeNodes.add(trafficHighlightedNode);
    const allLinks = topology.links || [];

    if (trafficHighlightedNode.startsWith('ip:')) {
      const activeModels = new Set();
      const activeChannels = new Set();
      for (const link of allLinks) {
        if (link.source === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
          activeModels.add(link.target);
        }
      }
      for (const link of allLinks) {
        if (activeModels.has(link.source)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
          activeChannels.add(link.target);
        }
      }
      for (const link of allLinks) {
        if (activeChannels.has(link.source)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
        }
      }
    } else if (trafficHighlightedNode.startsWith('model:')) {
      const activeIPs = new Set();
      const activeChannels = new Set();
      for (const link of allLinks) {
        if (link.target === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
          activeIPs.add(link.source);
        } else if (link.source === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
          activeChannels.add(link.target);
        }
      }
      for (const link of allLinks) {
        if (activeChannels.has(link.source)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
        }
      }
    } else if (trafficHighlightedNode.startsWith('channel:')) {
      const activeModels = new Set();
      for (const link of allLinks) {
        if (link.target === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
          activeModels.add(link.source);
        } else if (link.source === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.target);
        }
      }
      for (const link of allLinks) {
        if (activeModels.has(link.target)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
        }
      }
    } else if (trafficHighlightedNode.startsWith('status:')) {
      const activeChannels = new Set();
      const activeModels = new Set();
      for (const link of allLinks) {
        if (link.target === trafficHighlightedNode) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
          activeChannels.add(link.source);
        }
      }
      for (const link of allLinks) {
        if (activeChannels.has(link.target)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
          activeModels.add(link.source);
        }
      }
      for (const link of allLinks) {
        if (activeModels.has(link.target)) {
          activeLinks.add(`${link.source}|${link.target}`);
          activeNodes.add(link.source);
        }
      }
    }
  }

  // 3. 增量 Diff 更新 Links (平滑过渡 d 属性与透明度，绝不销毁 DOM)
  const seenLinkKeys = new Set();
  for (const link of (topology.links || [])) {
    const p1 = nodePositions[link.source];
    const p2 = nodePositions[link.target];
    if (!p1 || !p2) continue;

    const linkKey = `${link.source}|${link.target}`;
    seenLinkKeys.add(linkKey);

    const isLinkActive = !isFiltered || activeLinks.has(linkKey);
    const x1 = p1.x + nodeW;
    const y1 = p1.y + nodeH / 2;
    const x2 = p2.x;
    const y2 = p2.y + nodeH / 2;
    const dx = (x2 - x1) * 0.52;
    const d = `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;

    const isErr = link.target.includes('status:4') || link.target.includes('status:5');
    let strokeColor = isErr ? 'rgba(239, 68, 68, 0.45)' : 'rgba(59, 130, 246, 0.38)';
    let strokeW = Math.max(1.5, Math.min(7, Math.sqrt(link.value) * 1.5));

    if (isFiltered && isLinkActive) {
      strokeColor = isErr ? '#ef4444' : '#06b6d4';
      strokeW = Math.max(2.5, strokeW + 1.2);
    }

    let pathEl = linksLayer.querySelector(`path[data-link-key="${CSS.escape(linkKey)}"]`);
    if (!pathEl) {
      pathEl = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      pathEl.setAttribute('data-link-key', linkKey);
      pathEl.setAttribute('fill', 'none');
      linksLayer.appendChild(pathEl);
    }

    pathEl.setAttribute('d', d);
    pathEl.setAttribute('stroke', strokeColor);
    pathEl.setAttribute('stroke-width', String(strokeW));
    pathEl.setAttribute('class', `traffic-link-path ${isLinkActive ? 'is-active' : 'is-dimmed'}`);
  }

  // 移除过期 Links
  linksLayer.querySelectorAll('path[data-link-key]').forEach(el => {
    if (!seenLinkKeys.has(el.getAttribute('data-link-key'))) {
      el.remove();
    }
  });

  // 4. 增量 Diff 更新 Nodes (平滑插值 transform 坐标，更新计数，绝不销毁 DOM)
  const seenNodeIds = new Set();
  for (const [id, pos] of Object.entries(nodePositions)) {
    const item = pos.item;
    seenNodeIds.add(id);

    const isNodeActive = !isFiltered || activeNodes.has(id);
    const isNodeSelected = trafficHighlightedNode === id;
    const visual = getBrandNodeVisual(item.type, item.label);

    let shortLabel = item.label;
    if (shortLabel.length > 14) {
      shortLabel = shortLabel.slice(0, 13) + '…';
    }

    let subLocation = '';
    if (item.type === 'client_ip') {
      if (item.geo && (item.geo.formatted || item.geo.country)) {
        subLocation = item.geo.formatted || item.geo.country;
      } else if (item.label.startsWith('172.18.') || item.label.startsWith('172.17.')) {
        subLocation = 'Docker Gateway';
      } else if (item.label.startsWith('100.')) {
        subLocation = 'Tailnet Mesh';
      } else if (item.label === '127.0.0.1') {
        subLocation = '本机回环';
      }
      if (subLocation.length > 14) {
        subLocation = subLocation.slice(0, 13) + '…';
      }
    }

    let nodeGroup = nodesLayer.querySelector(`g[data-node-id="${CSS.escape(id)}"]`);
    if (!nodeGroup) {
      nodeGroup = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      nodeGroup.setAttribute('data-node-id', id);
      nodeGroup.setAttribute('onclick', `toggleHighlightTrafficNode('${escapeHtml(id)}')`);

      const textSvg = subLocation
        ? `<text class="node-title" x="36" y="18" fill="var(--text-main, #0f172a)" font-size="10.5" font-weight="700" font-family="var(--mono, monospace)">${escapeHtml(shortLabel)}</text>
           <text class="node-sub" x="36" y="30" fill="var(--text-muted, #64748b)" font-size="8.5" font-weight="500">${escapeHtml(subLocation)}</text>`
        : `<text class="node-title" x="36" y="24" fill="var(--text-main, #0f172a)" font-size="11" font-weight="600" font-family="var(--mono, monospace)">${escapeHtml(shortLabel)}</text>`;

      nodeGroup.innerHTML = `
        <rect class="node-box" width="${nodeW}" height="${nodeH}" rx="8" fill="${visual.bgColor}" stroke="${visual.borderColor}" stroke-width="1.2" />
        <svg class="node-icon" x="7" y="8" width="22" height="22" viewBox="0 0 16 16" overflow="visible" aria-hidden="true" focusable="false">
          ${visual.icon}
        </svg>
        ${textSvg}
        <rect x="${nodeW - 32}" y="10" width="24" height="18" rx="9" fill="rgba(15, 23, 42, 0.08)" />
        <text class="node-count-text" x="${nodeW - 20}" y="23" fill="var(--text-main, #0f172a)" font-size="9.5" font-weight="700" text-anchor="middle" font-family="var(--mono, monospace)">${item.count}</text>
      `;
      nodesLayer.appendChild(nodeGroup);
      const iconSvg = nodeGroup.querySelector(':scope > svg.node-icon');
      if (iconSvg) normalizeNodeIcon(iconSvg, visual.icon);
    } else {
      // 节点已存在：更新位置坐标与计数
      const countEl = nodeGroup.querySelector('.node-count-text');
      if (countEl) countEl.textContent = String(item.count);

      const subEl = nodeGroup.querySelector('.node-sub');
      if (subEl && subLocation) subEl.textContent = subLocation;
    }

    // 采用 CSS transform 平滑位移
    nodeGroup.setAttribute('transform', `translate(${pos.x}, ${pos.y})`);
    nodeGroup.setAttribute('class', `traffic-node-group ${isNodeActive ? '' : 'is-dimmed'} ${isNodeSelected ? 'is-selected' : ''}`);
  }

  // 移除过期 Nodes
  nodesLayer.querySelectorAll('g[data-node-id]').forEach(el => {
    if (!seenNodeIds.has(el.getAttribute('data-node-id'))) {
      el.remove();
    }
  });
}

// 7. 拓扑节点交互高亮追踪
function toggleHighlightTrafficNode(nodeId) {
  if (trafficHighlightedNode === nodeId) {
    trafficHighlightedNode = null;
    if (nodeId.startsWith('ip:')) {
      trafficSelectedIP = '';
      const ipSelect = document.getElementById('traffic-select-ip');
      if (ipSelect) ipSelect.value = '';
    }
  } else {
    trafficHighlightedNode = nodeId;
    if (nodeId.startsWith('ip:')) {
      const ip = nodeId.slice(3);
      trafficSelectedIP = ip;
      const ipSelect = document.getElementById('traffic-select-ip');
      if (ipSelect) ipSelect.value = ip;
    }
  }
  if (cachedTrafficData) {
    renderTrafficUI(cachedTrafficData, false);
  }
}

// 8. 渲染多用户关联视图 (IP · 属地 · Key 三维关联台账)
function renderTrafficIPLedger(ipStats) {
  const container = document.getElementById('traffic-ip-ledger-grid');
  if (!container) return;

  if (ipStats.length === 0) {
    container.innerHTML = `<div style="color:var(--text-muted);font-size:12px;padding:12px 0;">暂无活跃客户端 IP 记录</div>`;
    return;
  }

  container.innerHTML = ipStats.map(s => {
    const isSelected = trafficSelectedIP === s.ip;
    const timeStr = s.last_seen ? new Date(s.last_seen * 1000).toLocaleTimeString() : '-';
    const modelsBadge = (s.top_models || []).map(m => `<span class="traffic-pill traffic-pill-model">${escapeHtml(m)}</span>`).join(' ');

    let netBadge = '';
    if (s.ip === '172.18.0.1' || s.ip.startsWith('172.18.') || s.ip.startsWith('172.17.')) {
      netBadge = '<span style="font-size:10px;padding:1px 5px;border-radius:4px;background:rgba(2,132,199,0.12);color:#0284c7;font-weight:600;">Docker Bridge</span>';
    } else if (s.ip.startsWith('100.')) {
      netBadge = '<span style="font-size:10px;padding:1px 5px;border-radius:4px;background:rgba(139,92,246,0.12);color:#8b5cf6;font-weight:600;">Tailnet</span>';
    }

    // 属地与 ASN
    let locLine = '';
    if (s.geo && (s.geo.formatted || s.geo.country)) {
      const loc = s.geo.formatted || s.geo.country;
      const asn = s.geo.asn ? `AS${s.geo.asn} ${s.geo.asn_org || ''}`.trim() : (s.geo.asn_org || '');
      locLine = `
        <div style="display:flex;align-items:center;gap:6px;font-size:11px;color:var(--text-muted);margin:2px 0 4px 0;flex-wrap:wrap;">
          <span style="display:inline-flex;color:#0284c7;">${TRAFFIC_ICONS.mapPin}</span>
          <span style="font-weight:600;color:var(--text-main);">${escapeHtml(loc)}</span>
          ${asn ? `<span style="opacity:0.65;font-size:10px;">(${escapeHtml(asn)})</span>` : ''}
          ${s.geo.is_proxy ? '<span class="traffic-pill traffic-pill-danger" style="font-size:9px;padding:0 4px;">VPN/Proxy</span>' : ''}
        </div>
      `;
    }

    // 该 IP 名下所有使用过的 Keys 胶囊 (支持点击反查)
    let keysLine = '';
    if (Array.isArray(s.keys) && s.keys.length > 0) {
      const keyPills = s.keys.map(k => {
        const isKeyActive = trafficSelectedKey === k.key;
        return `
          <span class="traffic-pill traffic-pill-key ${isKeyActive ? 'is-active' : ''}" onclick="event.stopPropagation(); filterTrafficByKey('${escapeHtml(k.key)}')" title="点击反查使用此 Key 的所有关联 IP">
            Key: ${escapeHtml(k.key)} (${k.request_count}次 · ${(k.tokens || 0).toLocaleString()}tok)
          </span>
        `;
      }).join(' ');

      keysLine = `
        <div style="display:flex;gap:4px;flex-wrap:wrap;margin:4px 0;">
          ${keyPills}
        </div>
      `;
    }

    // 成功率与平均时延
    const succPct = ((s.success_rate || 0) * 100).toFixed(1);
    const avgLat = Math.round(s.avg_latency_ms || 0);

    return `
      <div class="traffic-ip-card ${isSelected ? 'is-active' : ''}" onclick="filterTrafficByIP('${escapeHtml(s.ip)}')">
        <div style="display:flex;justify-content:space-between;align-items:center;">
          <div style="display:flex;align-items:center;gap:7px;">
            <span class="traffic-pulse-dot"></span>
            <span style="font-family:var(--mono, monospace);font-weight:700;font-size:13px;color:${isSelected ? '#0891b2' : 'var(--text-main)'};">${escapeHtml(s.ip)}</span>
            ${netBadge}
          </div>
          <div style="display:flex;align-items:center;gap:8px;">
            <span style="font-size:11px;color:var(--text-muted);">${timeStr}</span>
            <span style="font-size:11px;color:#2563eb;cursor:pointer;font-weight:600;" onclick="event.stopPropagation(); jumpToLogsWithIP('${escapeHtml(s.ip)}')" title="在请求流水中查看此 IP 的所有调用明细">查看流水 ↗</span>
          </div>
        </div>
        ${locLine}
        ${keysLine}
        <div style="display:flex;gap:14px;font-size:11.5px;margin:4px 0;color:var(--text-muted);flex-wrap:wrap;">
          <span>请求: <strong style="color:var(--text-main);font-family:var(--mono, monospace);">${s.request_count}</strong></span>
          <span>Tokens: <strong style="color:#7c3aed;font-family:var(--mono, monospace);">${(s.tokens || 0).toLocaleString()}</strong></span>
          <span>成功率: <strong style="color:#059669;font-family:var(--mono, monospace);">${succPct}%</strong></span>
          <span>延迟: <strong style="color:var(--text-main);font-family:var(--mono, monospace);">${avgLat}ms</strong></span>
        </div>
        <div style="display:flex;gap:4px;flex-wrap:wrap;margin-top:2px;">${modelsBadge}</div>
      </div>
    `;
  }).join('');
}

// 8.5 渲染地域分布 Top 来源排行
function renderTrafficGeoDistribution(geoStats) {
  const container = document.getElementById('traffic-geo-distribution-grid');
  if (!container) return;

  if (!geoStats || geoStats.length === 0) {
    container.innerHTML = `<div style="color:var(--text-muted);font-size:12px;padding:12px 0;">暂无地域解析数据</div>`;
    return;
  }

  container.innerHTML = geoStats.slice(0, 6).map((g, idx) => {
    return `
      <div class="traffic-geo-card">
        <div style="display:flex;align-items:center;gap:8px;">
          <span class="traffic-geo-rank ${idx < 3 ? 'is-top' : ''}">${idx + 1}</span>
          <div style="display:flex;flex-direction:column;gap:1px;">
            <div style="display:flex;align-items:center;gap:5px;">
              <span style="color:#0284c7;display:inline-flex;">${TRAFFIC_ICONS.mapPin}</span>
              <span style="font-size:12px;font-weight:600;color:var(--text-main);">${escapeHtml(g.location)}</span>
            </div>
            <span style="font-size:10.5px;color:var(--text-muted);">${(g.tokens || 0).toLocaleString()} tokens</span>
          </div>
        </div>
        <div style="display:flex;align-items:center;gap:6px;">
          ${g.is_proxy ? '<span class="traffic-pill traffic-pill-warning" style="font-size:9px;padding:1px 4px;">Proxy</span>' : ''}
          <span style="font-family:var(--mono, monospace);font-weight:700;font-size:12px;color:#0284c7;">${g.request_count} 次</span>
        </div>
      </div>
    `;
  }).join('');
}

// 9. 更新时间范围滑动条极值
function updateTrafficTimeSliderLimits(records) {
  if (!records || records.length === 0) {
    trafficBufferMinTs = 0;
    trafficBufferMaxTs = 0;
    return;
  }

  let minTs = records[0].timestamp;
  let maxTs = records[0].timestamp;
  for (const r of records) {
    if (r.timestamp < minTs) minTs = r.timestamp;
    if (r.timestamp > maxTs) maxTs = r.timestamp;
  }

  trafficBufferMinTs = minTs;
  trafficBufferMaxTs = maxTs;

  const startLabel = document.getElementById('traffic-time-label-start');
  const endLabel = document.getElementById('traffic-time-label-end');
  if (startLabel) startLabel.textContent = minTs ? new Date(minTs * 1000).toLocaleTimeString() : '-';
  if (endLabel) endLabel.textContent = maxTs ? new Date(maxTs * 1000).toLocaleTimeString() : '-';

  updateTrafficSliderProgressUI();
}

// 10. 时间滑动条输入与快捷预设处理
function onTrafficTimeSliderInput() {
  trafficActiveTimePreset = 'custom';
  document.querySelectorAll('.traffic-preset-btn').forEach(b => b.classList.remove('active'));

  const minInput = document.getElementById('traffic-time-min');
  const maxInput = document.getElementById('traffic-time-max');
  if (!minInput || !maxInput) return;

  let minVal = Number(minInput.value);
  let maxVal = Number(maxInput.value);

  if (minVal > maxVal) {
    const tmp = minVal;
    minVal = maxVal;
    maxVal = tmp;
  }

  trafficTimeMinPercent = minVal;
  trafficTimeMaxPercent = maxVal;

  updateTrafficSliderProgressUI();
  if (cachedTrafficData) {
    renderTrafficTable(cachedTrafficData.records || cachedTrafficData.recent_requests || []);
  }
}

function updateTrafficSliderProgressUI() {
  const progress = document.getElementById('traffic-slider-progress');
  const currentLabel = document.getElementById('traffic-time-label-current');
  if (!progress) return;

  progress.style.left = `${trafficTimeMinPercent}%`;
  progress.style.right = `${100 - trafficTimeMaxPercent}%`;

  if (currentLabel) {
    if (trafficTimeMinPercent === 0 && trafficTimeMaxPercent === 100) {
      currentLabel.textContent = '全量跨度';
    } else if (trafficBufferMinTs > 0 && trafficBufferMaxTs > 0) {
      const span = trafficBufferMaxTs - trafficBufferMinTs;
      const startT = Math.round(trafficBufferMinTs + span * (trafficTimeMinPercent / 100));
      const endT = Math.round(trafficBufferMinTs + span * (trafficTimeMaxPercent / 100));
      currentLabel.textContent = `${new Date(startT * 1000).toLocaleTimeString()} ➔ ${new Date(endT * 1000).toLocaleTimeString()}`;
    }
  }
}

function setTrafficTimePreset(preset) {
  trafficActiveTimePreset = preset;
  document.querySelectorAll('.traffic-preset-btn').forEach(b => {
    b.classList.toggle('active', b.id === `btn-preset-${preset}`);
  });

  const minInput = document.getElementById('traffic-time-min');
  const maxInput = document.getElementById('traffic-time-max');
  if (!minInput || !maxInput) return;

  if (preset === 'all' || trafficBufferMaxTs <= trafficBufferMinTs) {
    trafficTimeMinPercent = 0;
    trafficTimeMaxPercent = 100;
  } else {
    const nowTs = Math.floor(Date.now() / 1000);
    const span = Math.max(1, trafficBufferMaxTs - trafficBufferMinTs);
    let durationSec = 300; // 5m default
    if (preset === '1h') durationSec = 3600;
    else if (preset === '15m') durationSec = 900;
    else if (preset === '5m') durationSec = 300;

    const targetStartTs = Math.max(trafficBufferMinTs, trafficBufferMaxTs - durationSec);
    trafficTimeMinPercent = Math.max(0, Math.min(100, Math.round(((targetStartTs - trafficBufferMinTs) / span) * 100)));
    trafficTimeMaxPercent = 100;
  }

  minInput.value = String(trafficTimeMinPercent);
  maxInput.value = String(trafficTimeMaxPercent);

  updateTrafficSliderProgressUI();
  if (cachedTrafficData) {
    renderTrafficTable(cachedTrafficData.records || cachedTrafficData.recent_requests || []);
  }
}

// 11. 渲染实时调用流水明细表格 (10 列规范对齐)
function renderTrafficTable(records) {
  const tbody = document.getElementById('traffic-tbody');
  const countEl = document.getElementById('traffic-records-count');
  if (!tbody) return;

  // 计算滑动条时间窗口
  let filterStartTs = 0;
  let filterEndTs = 0;
  if (trafficBufferMinTs > 0 && trafficBufferMaxTs > 0) {
    const span = trafficBufferMaxTs - trafficBufferMinTs;
    filterStartTs = trafficBufferMinTs + span * (trafficTimeMinPercent / 100);
    filterEndTs = trafficBufferMinTs + span * (trafficTimeMaxPercent / 100);
  }

  const filtered = records.filter(r => {
    // 时间范围滑动条过滤
    if (filterStartTs > 0 && filterEndTs > 0) {
      if (r.timestamp < filterStartTs || r.timestamp > filterEndTs) return false;
    }

    if (trafficSelectedIP && r.client_ip !== trafficSelectedIP) return false;
    if (trafficSelectedKey && r.key_label !== trafficSelectedKey) return false;
    if (trafficSelectedModel && r.model !== trafficSelectedModel && r.request_model !== trafficSelectedModel) return false;
    if (trafficSelectedStatus !== 'all') {
      if (trafficSelectedStatus === '2xx' && (r.status < 200 || r.status >= 300)) return false;
      if (trafficSelectedStatus === '4xx' && (r.status < 400 || r.status >= 500)) return false;
      if (trafficSelectedStatus === '5xx' && r.status < 500) return false;
    }
    if (trafficSearchQuery) {
      const q = trafficSearchQuery.toLowerCase();
      const mIP = r.client_ip && r.client_ip.toLowerCase().includes(q);
      const mModel = r.model && r.model.toLowerCase().includes(q);
      const mReqModel = r.request_model && r.request_model.toLowerCase().includes(q);
      const mKey = r.key_label && r.key_label.toLowerCase().includes(q);
      const mChan = r.channel && r.channel.toLowerCase().includes(q);
      const mPath = r.path && r.path.toLowerCase().includes(q);
      const mGeo = r.geo && ((r.geo.formatted && r.geo.formatted.toLowerCase().includes(q)) || (r.geo.country && r.geo.country.toLowerCase().includes(q)) || (r.geo.region && r.geo.region.toLowerCase().includes(q)) || (r.geo.city && r.geo.city.toLowerCase().includes(q)) || (r.geo.asn_org && r.geo.asn_org.toLowerCase().includes(q)));
      if (!mIP && !mModel && !mReqModel && !mKey && !mChan && !mPath && !mGeo) return false;
    }
    return true;
  });

  if (countEl) {
    countEl.textContent = `展示 ${filtered.length} / ${records.length} 条`;
  }

  if (filtered.length === 0) {
    tbody.innerHTML = `<tr><td colspan="10" style="text-align:center;padding:36px;color:var(--text-muted);font-size:12.5px;">无匹配的调用记录</td></tr>`;
    return;
  }

  tbody.innerHTML = filtered.slice(0, 100).map(r => {
    const isSuccess = r.status >= 200 && r.status < 300;
    const is4xx = r.status >= 400 && r.status < 500;
    const statusClass = isSuccess ? 'traffic-pill-success' : is4xx ? 'traffic-pill-warning' : 'traffic-pill-danger';
    const timeStr = r.timestamp ? new Date(r.timestamp * 1000).toLocaleTimeString() : '-';

    // 属地与 ASN
    let locText = (r.geo && (r.geo.formatted || r.geo.country))
      ? (r.geo.formatted || r.geo.country)
      : (r.client_ip === '172.18.0.1' || r.client_ip.startsWith('172.18.') ? 'Docker Gateway' : (r.client_ip.startsWith('100.') ? 'Tailnet 专网' : (r.client_ip === '127.0.0.1' ? '本机回环' : '未知地域')));
    const isProxy = Boolean(r.geo && (r.geo.is_proxy || r.geo.is_vpn || r.geo.is_tor));
    const asnText = r.geo && r.geo.asn ? `AS${r.geo.asn} ${r.geo.asn_org || ''}`.trim() : (r.geo?.asn_org || '-');

    // 如果客户端请求模型与实际路由执行模型不同，展示真实透传流向
    let modelHtml = '';
    if (r.request_model && r.request_model !== r.model) {
      modelHtml = `<span style="opacity:0.8;" title="客户端请求: ${escapeHtml(r.request_model)}">${escapeHtml(r.request_model)}</span> <span style="color:var(--text-muted);">${TRAFFIC_ICONS.arrowRight}</span> <strong style="color:#7c3aed;">${escapeHtml(r.model || '-')}</strong>`;
    } else {
      modelHtml = `<span style="font-weight:600;">${escapeHtml(r.model || '-')}</span>`;
    }
    if (r.path && r.path !== '/v1/chat/completions' && r.path !== '/v1/messages') {
      modelHtml += ` <span style="font-size:10px;color:var(--text-muted);opacity:0.75;">(${escapeHtml(r.path)})</span>`;
    }

    return `
      <tr style="border-bottom:1px solid var(--border-subtle, rgba(0,0,0,0.05));">
        <td style="padding:10px 12px;font-family:var(--mono, monospace);color:var(--text-muted);">${timeStr}</td>
        <td style="padding:10px 12px;">
          <span class="traffic-pill traffic-pill-ip" onclick="filterTrafficByIP('${escapeHtml(r.client_ip)}')" title="点击筛选此 IP">${escapeHtml(r.client_ip || '-')}</span>
        </td>
        <td style="padding:10px 12px;">
          <span class="traffic-pill traffic-pill-geo">${escapeHtml(locText)}</span>
          ${isProxy ? ' <span class="traffic-pill traffic-pill-danger" style="font-size:9px;padding:0 4px;">Proxy</span>' : ''}
        </td>
        <td style="padding:10px 12px;color:var(--text-muted);font-size:11px;font-family:var(--mono, monospace);max-width:140px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${escapeHtml(asnText)}">${escapeHtml(asnText)}</td>
        <td style="padding:10px 12px;">
          <span class="traffic-pill traffic-pill-key ${trafficSelectedKey === r.key_label ? 'is-active' : ''}" onclick="filterTrafficByKey('${escapeHtml(r.key_label)}')" title="点击反查使用此 Key 的所有 IP">${escapeHtml(r.key_label || '-')}</span>
        </td>
        <td style="padding:10px 12px;"><span class="traffic-pill traffic-pill-model">${modelHtml}</span></td>
        <td style="padding:10px 12px;"><span class="traffic-pill traffic-pill-channel">${escapeHtml(r.channel || '-')}</span></td>
        <td style="padding:10px 12px;"><span class="traffic-pill ${statusClass}">${r.status || 200}</span></td>
        <td style="padding:10px 12px;font-family:var(--mono, monospace);font-variant-numeric:tabular-nums;color:${r.latency_ms > 3000 ? '#d97706' : 'inherit'};">${r.latency_ms || 0}ms</td>
        <td style="padding:10px 12px;font-family:var(--mono, monospace);font-variant-numeric:tabular-nums;color:#7c3aed;">${(r.tokens || 0).toLocaleString()}</td>
      </tr>
    `;
  }).join('');
}

// 12. 更新下拉筛选菜单
function updateTrafficFilterDropdowns(records) {
  const ipSelect = document.getElementById('traffic-select-ip');
  const modelSelect = document.getElementById('traffic-select-model');
  if (!ipSelect || !modelSelect) return;

  const currentIP = trafficSelectedIP;
  const currentModel = trafficSelectedModel;

  const ipSet = new Set();
  const modelSet = new Set();
  for (const r of records) {
    if (r.client_ip) ipSet.add(r.client_ip);
    if (r.model) modelSet.add(r.model);
    if (r.request_model) modelSet.add(r.request_model);
  }

  const ips = Array.from(ipSet).sort();
  const models = Array.from(modelSet).sort();

  ipSelect.innerHTML = `<option value="">全部客户端 IP</option>` + ips.map(ip => `<option value="${escapeHtml(ip)}" ${ip === currentIP ? 'selected' : ''}>${escapeHtml(ip)}</option>`).join('');
  modelSelect.innerHTML = `<option value="">全部请求模型</option>` + models.map(m => `<option value="${escapeHtml(m)}" ${m === currentModel ? 'selected' : ''}>${escapeHtml(m)}</option>`).join('');
}

function filterTrafficByIP(ip) {
  trafficSelectedIP = (trafficSelectedIP === ip) ? '' : ip;
  trafficHighlightedNode = trafficSelectedIP ? `ip:${trafficSelectedIP}` : null;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function filterTrafficByKey(keyName) {
  trafficSelectedKey = (trafficSelectedKey === keyName) ? '' : keyName;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

window.jumpToLogsWithIP = function(ip) {
  if (typeof switchNavTab === 'function') {
    switchNavTab('logs');
  }
  if (typeof filterLogsByIP === 'function') {
    filterLogsByIP(ip);
  }
};

function onTrafficIPChange(val) {
  trafficSelectedIP = val;
  trafficHighlightedNode = val ? `ip:${val}` : null;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function onTrafficModelChange(val) {
  trafficSelectedModel = val;
  trafficHighlightedNode = val ? `model:${val}` : null;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function onTrafficStatusChange(val) {
  trafficSelectedStatus = val;
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function onTrafficSearchChange(val) {
  trafficSearchQuery = val.trim();
  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function resetTrafficFilters() {
  trafficSelectedIP = '';
  trafficSelectedKey = '';
  trafficSelectedModel = '';
  trafficSelectedStatus = 'all';
  trafficSearchQuery = '';
  trafficHighlightedNode = null;
  setTrafficTimePreset('all');

  const ipSel = document.getElementById('traffic-select-ip');
  const mSel = document.getElementById('traffic-select-model');
  const stSel = document.getElementById('traffic-select-status');
  const sInput = document.getElementById('traffic-search-input');

  if (ipSel) ipSel.value = '';
  if (mSel) mSel.value = '';
  if (stSel) stSel.value = 'all';
  if (sInput) sInput.value = '';

  if (cachedTrafficData) renderTrafficUI(cachedTrafficData, false);
}

function setTrafficPollingRate(rate) {
  trafficPollingInterval = Number(rate);
  if (trafficTimer) {
    clearInterval(trafficTimer);
    trafficTimer = null;
  }
  if (trafficPollingInterval > 0) {
    trafficTimer = setInterval(() => {
      if (typeof activeNavTab !== 'undefined' && activeNavTab === 'traffic') {
        fetchTrafficData();
      }
    }, trafficPollingInterval * 1000);
  }
}

// 页面加载与 SWR 初次直出
document.addEventListener('DOMContentLoaded', () => {
  setTrafficPollingRate(3);
  try {
    const cachedRaw = sessionStorage.getItem(TRAFFIC_SWR_CACHE_KEY);
    if (cachedRaw) {
      const cached = JSON.parse(cachedRaw);
      if (cached && cached.summary) {
        cachedTrafficData = cached;
        renderTrafficUI(cached, false);
      }
    }
  } catch (e) {}
});
