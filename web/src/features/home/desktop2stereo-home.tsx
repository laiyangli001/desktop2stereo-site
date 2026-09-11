import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  Check,
  Download,
  ExternalLink,
  Image as ImageIcon,
  Monitor,
  PlayCircle,
  QrCode,
  Sparkles,
  X,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconGithub } from '@/assets/brand-icons'
import { Button } from '@/components/ui/button'

interface Desktop2StereoHomeProps {
  isAuthenticated: boolean
}

const repoUrl = 'https://github.com/lc700x/desktop2stereo'
const releaseUrl = `${repoUrl}/releases/tag/v2.5.0`
const requirementsUrl = `${repoUrl}/blob/main/requirements.txt`
const guideUrl = `${repoUrl}#readme`
const bilibiliGuideUrl = 'https://space.bilibili.com/156928642/lists'
const quarkUrl = 'https://pan.quark.cn/s/9d2bcf039b96'
const baiduUrl = 'https://pan.baidu.com/s/1JfMpcrdplPTQSNB6rExh6A?pwd=mr64'
const external = (href: string) => ({
  href,
  target: '_blank',
  rel: 'noreferrer',
})

const copy = {
  en: {
    eyebrow: 'Official Desktop2Stereo Website',
    hero: 'Convert ordinary desktop, video, game, and media content into stereo 3D output for large displays, VR viewing, and 3D display experiments.',
    download: 'Download v2.5',
    guide: 'Read install guide',
    dashboard: 'Go to Dashboard',
    signals: [
      'Official GitHub project',
      'Source and lazy packages',
      'OpenXR ready',
      'Chinese and English resources',
    ],
    intro:
      'Cross-vendor hardware, multiple operating systems, and several output paths for real 2D-to-3D workflows.',
    modes: [
      'NVIDIA / AMD / Intel',
      'Apple Silicon',
      'CUDA / ROCm',
      'GPU zero-copy',
      'OpenXR',
      'Network streaming',
    ],
    compatibility: 'Compatibility',
    compatibilityTitle: 'Supported hardware and operating systems',
    compatibilityCopy:
      'Desktop2Stereo is designed for cross-vendor GPU acceleration and mainstream desktop operating systems.',
    hardware: 'Hardware',
    systems: 'Operating systems',
    hardwareItems: [
      'NVIDIA GPU',
      'AMD GPU',
      'Intel GPU',
      'Apple Silicon M1–M4',
      'DirectML devices on Windows',
    ],
    systemItems: [
      'Windows 10/11 (x64 / Arm64)',
      'macOS 10.16 or later',
      'Ubuntu 22.04 or later',
    ],
    features: 'Features',
    featuresTitle:
      'Broad hardware support, tunable stereo parameters, multiple 3D outputs.',
    featuresCopy:
      'A layered real-time pipeline captures input, estimates depth, synthesizes stereo eyes, and presents the result to local display, OpenXR, stream, export, or API targets.',
    featureItems: [
      [
        'Unified capture input',
        'Monitor, window, image, video, and API frames enter one real-time stereoscopic synthesis pipeline.',
      ],
      [
        'GPU resident pipeline',
        'Capture, depth, synthesis, hole fill, temporal, and pack stages stay on CUDA / ROCm / MPS / XPU where possible.',
      ],
      [
        'Local, OpenXR, and streaming',
        'Run locally, use OpenXR Link with a headset, or send stereo output through MJPEG / RTMP / HLS.',
      ],
      [
        'Multiple 3D output formats',
        'Half-SBS, Full-SBS, Half-TAB, Full-TAB, Depth Map, Anaglyph, Interleaved, Mono, and Leia output.',
      ],
      [
        'Tunable stereo controls',
        'Adjust parallax, depth strength, convergence, separation, edge, hole fill, temporal, render scale, and FPS.',
      ],
      [
        'Real-time depth pipeline',
        'Choose depth models and resolution, enable FP16 acceleration, stabilize video temporally, and reset scenes.',
      ],
      [
        'China-friendly distribution',
        'Use the GitHub source package or the lazy package with a complete Python runtime environment.',
      ],
    ],
    parameters: [
      [
        'Runtime targets',
        'local_display, network_stream, openxr, debug_export, headless_api, auto',
      ],
      [
        'Synthesis modes',
        'rgb_depth_direct, full_synthesis_eyes, packed_synthesis',
      ],
      [
        'Output transports',
        'local_window, local_fullscreen, encoded_stream, openxr_swapchain, file_export, api_result',
      ],
      [
        'Stereo presets',
        'Traditional / Fastest, Cinema, Game / Low Latency, Image / High Quality',
      ],
      [
        'Core depth controls',
        'Parallax Budget, Depth Strength, Convergence, Dynamic Convergence, Depth Separation',
      ],
      [
        'Edge and fill controls',
        'Edge Threshold, Edge Dilation, Mask Feather, Hole Fill Mode, Depth Antialias',
      ],
    ],
    downloadTitle: 'Two download paths',
    downloadCopy:
      'Mainland China users should usually start with the lazy package. Other users can start from GitHub source and install Python requirements manually.',
    outside: 'Outside mainland China',
    sourceTitle: 'Source package from GitHub',
    sourceCopy:
      'Download the repository source with requirements.txt, then create and install the Python runtime environment yourself.',
    release: 'Open v2.5.0 releases',
    requirements: 'requirements.txt',
    mainland: 'Mainland China',
    lazyTitle: 'Lazy package with Python runtime',
    lazyCopy:
      'The prepackaged archive includes a complete Python runtime environment to reduce setup friction.',
    quark: 'Quark Netdisk',
    baidu: 'Baidu Netdisk',
    codes: 'Extraction codes: Quark 1vcn · Baidu mr64',
    guideTitle: 'Use the official README as the operating manual.',
    guideCopy:
      'The README in lc700x/desktop2stereo is the canonical source for hardware requirements, dependency installation, quick start, model selection, and troubleshooting.',
    webGuide: 'Open web guide',
    video: 'Bilibili video tutorials',
    workflow: 'Workflow',
    workflowTitle: 'From source to stereo output.',
    steps: [
      [
        'Download',
        'Pick the GitHub source or China lazy package for your region and setup preference.',
      ],
      [
        'Install',
        'Create the Python environment and install requirements.txt as described in the README.',
      ],
      [
        'Run',
        'Follow the quick-run instructions and choose the proper mode for desktop, video, or game content.',
      ],
      [
        'Tune',
        'Adjust stereo strength, depth model, display mode, and runtime settings for your hardware.',
      ],
    ],
    platforms: 'Platforms',
    platformsTitle: 'Version and project links',
    platformsCopy:
      'Current website version label: Desktop2Stereo v2.5. The official code and tutorial source is lc700x/desktop2stereo.',
    platformItems: [
      [
        'Official GitHub',
        'Repository, README, source files, requirements, and issue history.',
      ],
      [
        'v2.5 software line',
        'Website copy follows the current release information.',
      ],
      [
        'China downloads',
        'Quark and Baidu lazy packages with a complete runtime.',
      ],
      ['Manual source install', 'GitHub ZIP plus Python dependency setup.'],
    ],
    openGithub: 'Open official GitHub',
    community: 'Community',
    communityTitle: 'Join the official QQ discussion group.',
    communityCopy:
      'For usage questions, troubleshooting, and Chinese community discussion, scan the QR code to join the D2S official QQ group.',
    enlarge: 'Enlarge QR code',
    support: 'Support',
    supportTitle: 'Support Desktop2Stereo',
    supportCopy:
      'Turning 2D video into real-time 3D in VR is expensive to maintain. Sponsorship helps fund monthly model tokens, distribution storage, and continued image-quality work.',
    supportNote:
      'If sponsorship is not convenient, a free GitHub Star or sharing the project in a VR group helps just as much.',
    project: 'Project information',
    projectItems: [
      'Current software version: Desktop2Stereo v2.5',
      'Official source: lc700x/desktop2stereo on GitHub',
      'Manual resources: English / 中文',
    ],
    close: 'Close',
  },
  zh: {
    eyebrow: '官方 Desktop2Stereo 网站',
    hero: 'Desktop2Stereo v2.5 可以把普通桌面、视频、游戏和媒体内容转换成 3D 立体输出，面向大屏、VR 观看和 3D 显示实验。',
    download: '下载 v2.5',
    guide: '查看使用教程',
    dashboard: '进入控制台',
    signals: [
      '官方 GitHub 项目',
      '源码版与懒人包',
      '支持 OpenXR',
      '中英文资源',
    ],
    intro:
      '支持多厂商硬件、多操作系统和多种输出链路，面向真实 2D 转 3D 工作流。',
    modes: [
      'NVIDIA / AMD / Intel',
      'Apple Silicon',
      'CUDA / ROCm',
      'GPU 零拷贝',
      'OpenXR',
      '网络推流',
    ],
    compatibility: '兼容性',
    compatibilityTitle: '支持硬件与支持系统',
    compatibilityCopy:
      'Desktop2Stereo 面向多厂商 GPU 加速和主流桌面操作系统设计。',
    hardware: '支持硬件',
    systems: '支持系统',
    hardwareItems: [
      'NVIDIA GPU',
      'AMD GPU',
      'Intel GPU',
      'Apple Silicon 芯片（M1、M2、M3、M4 等）',
      '其他 DirectML 设备（仅 Windows）',
    ],
    systemItems: [
      'Windows 10/11（x64 / Arm64）',
      'macOS 10.16 或更高版本',
      'Ubuntu 22.04 或更高版本',
    ],
    features: '功能',
    featuresTitle: '硬件覆盖广，立体参数可调，3D 输出格式多。',
    featuresCopy:
      'Desktop2Stereo 按分层实时 2D 转 3D 管线设计：捕获输入、估计深度、合成左右眼，再呈现到本地显示、OpenXR、推流、导出或 API 目标。',
    featureItems: [
      [
        '统一捕获输入',
        '显示器、窗口、图片、视频和 API 帧都进入同一套实时立体视觉合成管线。',
      ],
      [
        'GPU 驻留管线',
        '捕获、深度、合成、补洞、时域稳定和封装阶段尽量保持在 CUDA / ROCm / MPS / XPU 中。',
      ],
      [
        '本地、OpenXR 与推流',
        '可本地运行，也可通过 OpenXR Link 进入 VR 头显，或通过 MJPEG / RTMP / HLS 推流。',
      ],
      [
        '多种 3D 输出格式',
        '支持 Half-SBS、Full-SBS、Half-TAB、Full-TAB、Depth Map、Anaglyph、Interleaved、Mono 和 Leia。',
      ],
      [
        '立体效果参数',
        '可调视差、深度强度、会聚、分离、边缘、补洞、时域、渲染比例和 FPS。',
      ],
      [
        '实时深度流水线',
        '支持深度模型、分辨率、FP16 加速、视频时域稳定和场景重置。',
      ],
      [
        '中国大陆友好分发',
        '可使用 GitHub 源码版，或使用带完整 Python 运行环境的懒人包。',
      ],
    ],
    parameters: [
      [
        '运行目标',
        'local_display、network_stream、openxr、debug_export、headless_api、auto',
      ],
      [
        '立体合成模式',
        'rgb_depth_direct、full_synthesis_eyes、packed_synthesis',
      ],
      [
        '输出传输',
        'local_window、local_fullscreen、encoded_stream、openxr_swapchain、file_export、api_result',
      ],
      [
        '立体预设',
        'Traditional / Fastest、Cinema、Game / Low Latency、Image / High Quality',
      ],
      [
        '核心深度控制',
        'Parallax Budget、Depth Strength、Convergence、Dynamic Convergence、Depth Separation',
      ],
      [
        '边缘与补洞控制',
        'Edge Threshold、Edge Dilation、Mask Feather、Hole Fill Mode、Depth Antialias',
      ],
    ],
    downloadTitle: '两种下载方式',
    downloadCopy:
      '中国大陆用户通常优先选择懒人包；其他用户可以从 GitHub 下载源码并自行配置 Python 运行环境。',
    outside: '中国大陆以外',
    sourceTitle: 'GitHub 源码版',
    sourceCopy:
      '下载包含 requirements.txt 的仓库源码，自行配置 Python 运行环境。',
    release: '打开 v2.5.0 Release',
    requirements: 'requirements.txt',
    mainland: '中国大陆',
    lazyTitle: '带 Python 环境的懒人包',
    lazyCopy: '压缩包已包含完整 Python 运行环境，减少依赖安装和环境配置成本。',
    quark: '夸克网盘',
    baidu: '百度网盘',
    codes: '提取码：夸克 1vcn · 百度 mr64',
    guideTitle: '使用教程以官方 README 为准。',
    guideCopy:
      'lc700x/desktop2stereo 仓库的 README 是官方教程来源，包含硬件要求、依赖安装、快速启动、模型选择和常见问题。',
    webGuide: '打开网页教程',
    video: 'B站视频教程',
    workflow: '工作流程',
    workflowTitle: '从源码到立体输出。',
    steps: [
      ['下载', '根据地区和安装能力选择 GitHub 源码版或中国大陆懒人包。'],
      ['安装', '按照 README 创建 Python 环境并安装 requirements.txt。'],
      ['运行', '按快速运行说明选择桌面、视频或游戏内容对应的模式。'],
      ['调试', '根据硬件调整立体强度、深度模型、显示模式和运行参数。'],
    ],
    platforms: '项目链接',
    platformsTitle: '版本与项目链接',
    platformsCopy:
      '当前网站标注的软件版本为 Desktop2Stereo v2.5，官方代码与教程来源是 lc700x/desktop2stereo。',
    platformItems: [
      [
        '官方 GitHub',
        '仓库、README 教程、源码文件、requirements.txt 和 issue 历史。',
      ],
      ['v2.5 软件线', '网站文案按当前提供的版本信息标注。'],
      ['中国下载', '夸克与百度网盘懒人包，包含完整运行库。'],
      ['手动源码安装', 'GitHub ZIP 加 Python 依赖配置。'],
    ],
    openGithub: '打开官方 GitHub',
    community: '社区',
    communityTitle: '加入 D2S 官方 QQ 问题讨论群。',
    communityCopy:
      '使用问题、故障排查和中文社区交流，可以扫码加入 D2S 官方 QQ 问题讨论群。',
    enlarge: '放大二维码',
    support: '赞助',
    supportTitle: '赞助 Desktop2Stereo',
    supportCopy:
      '让 2D 视频在 VR 里变成实时 3D 巨幕需要持续投入。赞助将用于模型 Token、网盘存储和后续画质提升。',
    supportNote:
      '如果不方便赞助，点个免费的 GitHub Star 或在 VR 群里转发项目同样有帮助。',
    project: '项目信息',
    projectItems: [
      '当前软件版本：Desktop2Stereo v2.5',
      '官方源码：GitHub lc700x/desktop2stereo',
      '手册资源：English / 中文',
    ],
    close: '关闭',
  },
} as const

export function Desktop2StereoHome({
  isAuthenticated,
}: Desktop2StereoHomeProps) {
  const { i18n } = useTranslation()
  const text =
    i18n.language === 'zhCN' || i18n.language.startsWith('zh')
      ? copy.zh
      : copy.en
  const [lightbox, setLightbox] = useState<'qq' | 'support' | null>(null)
  return (
    <main className='d2s-home bg-background text-foreground'>
      <section className='relative isolate overflow-hidden border-b px-6 py-20 md:py-28'>
        <picture className='absolute inset-0 -z-20 block h-full w-full'>
          <source srcSet='/d2s_full.jpg' type='image/jpeg' />
          <img
            src='/d2s_full.png'
            alt='Desktop2Stereo stereo desktop workstation'
            className='h-full w-full object-cover object-center opacity-25 dark:opacity-35'
          />
        </picture>
        <div className='from-background via-background/95 to-background/60 absolute inset-0 -z-10 bg-gradient-to-r' />
        <div className='mx-auto grid max-w-6xl items-center gap-12 lg:grid-cols-[1.05fr_.95fr]'>
          <div>
            <p className='text-primary mb-4 text-sm font-semibold tracking-[.18em] uppercase'>
              {text.eyebrow}
            </p>
            <h1 className='max-w-3xl text-4xl font-bold tracking-tight md:text-6xl'>
              Desktop2Stereo <span className='text-primary'>v2.5</span>
            </h1>
            <p className='text-muted-foreground mt-6 max-w-2xl text-lg leading-8'>
              {text.hero}
            </p>
            <div className='mt-8 flex flex-wrap gap-3'>
              <Button render={<a {...external(releaseUrl)} />}>
                <Download className='mr-2 size-4' />
                {text.download}
              </Button>
              <Button variant='outline' render={<a {...external(guideUrl)} />}>
                <PlayCircle className='mr-2 size-4' />
                {text.guide}
              </Button>
              {isAuthenticated ? (
                <Button variant='ghost' render={<Link to='/dashboard' />}>
                  {text.dashboard}
                  <ArrowRight className='ml-2 size-4' />
                </Button>
              ) : null}
            </div>
            <div className='text-muted-foreground mt-8 flex flex-wrap gap-2 text-sm'>
              {text.signals.map((item) => (
                <span
                  key={item}
                  className='bg-background/70 rounded-full border px-3 py-1.5'
                >
                  {item}
                </span>
              ))}
            </div>
          </div>
          <div className='bg-card/80 rounded-2xl border p-3 shadow-xl backdrop-blur-sm'>
            <picture>
              <source srcSet='/d2s_full.jpg' type='image/jpeg' />
              <img
                src='/d2s_full.png'
                alt='Desktop2Stereo depth and stereo preview'
                className='aspect-[4/3] w-full rounded-xl object-cover'
              />
            </picture>
          </div>
        </div>
      </section>
      <section className='border-b px-6 py-10'>
        <div className='mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4'>
          <p className='text-muted-foreground max-w-xl text-lg'>{text.intro}</p>
          <div className='flex max-w-2xl flex-wrap justify-end gap-2 text-sm'>
            {text.modes.map((item) => (
              <span key={item} className='bg-muted rounded-md px-3 py-2'>
                {item}
              </span>
            ))}
          </div>
        </div>
      </section>
      <section id='compatibility' className='mx-auto max-w-6xl px-6 py-20'>
        <div className='mb-10 max-w-3xl'>
          <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
            {text.compatibility}
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            {text.compatibilityTitle}
          </h2>
          <p className='text-muted-foreground mt-4'>{text.compatibilityCopy}</p>
        </div>
        <div className='grid gap-4 md:grid-cols-2'>
          <article className='bg-card rounded-xl border p-6 shadow-sm'>
            <h3 className='mb-4 flex items-center gap-2 text-xl font-medium'>
              <Monitor className='text-primary size-5' />
              {text.hardware}
            </h3>
            <ul className='text-muted-foreground space-y-3'>
              {text.hardwareItems.map((item) => (
                <li key={item} className='flex gap-2'>
                  <Check className='text-primary mt-0.5 size-4 shrink-0' />
                  {item}
                </li>
              ))}
            </ul>
          </article>
          <article className='bg-card rounded-xl border p-6 shadow-sm'>
            <h3 className='mb-4 flex items-center gap-2 text-xl font-medium'>
              <Monitor className='text-primary size-5' />
              {text.systems}
            </h3>
            <ul className='text-muted-foreground space-y-3'>
              {text.systemItems.map((item) => (
                <li key={item} className='flex gap-2'>
                  <Check className='text-primary mt-0.5 size-4 shrink-0' />
                  {item}
                </li>
              ))}
            </ul>
          </article>
        </div>
      </section>
      <section id='capabilities' className='bg-muted/20 border-y px-6 py-20'>
        <div className='mx-auto max-w-6xl'>
          <div className='mb-10 max-w-3xl'>
            <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
              {text.features}
            </p>
            <h2 className='text-3xl font-semibold md:text-4xl'>
              {text.featuresTitle}
            </h2>
            <p className='text-muted-foreground mt-4'>{text.featuresCopy}</p>
          </div>
          <div className='grid gap-4 md:grid-cols-2 lg:grid-cols-3'>
            {text.featureItems.map(([title, description]) => (
              <article
                key={title}
                className='bg-card rounded-xl border p-6 shadow-sm transition-transform hover:-translate-y-1'
              >
                <div className='bg-primary/10 text-primary mb-5 flex size-10 items-center justify-center rounded-lg'>
                  <Sparkles className='size-5' />
                </div>
                <h3 className='text-lg font-medium'>{title}</h3>
                <p className='text-muted-foreground mt-3 text-sm leading-6'>
                  {description}
                </p>
              </article>
            ))}
          </div>
          <div className='mt-10 grid gap-4 md:grid-cols-2 lg:grid-cols-3'>
            {text.parameters.map(([title, description]) => (
              <article
                key={title}
                className='bg-background rounded-xl border p-5'
              >
                <h3 className='font-medium'>{title}</h3>
                <p className='text-muted-foreground mt-2 font-mono text-xs leading-5 break-words'>
                  {description}
                </p>
              </article>
            ))}
          </div>
        </div>
      </section>
      <section id='download' className='mx-auto max-w-6xl px-6 py-20'>
        <div className='mb-10 max-w-3xl'>
          <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
            Download / 下载
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            {text.downloadTitle}
          </h2>
          <p className='text-muted-foreground mt-4'>{text.downloadCopy}</p>
        </div>
        <div className='grid gap-4 md:grid-cols-2'>
          <article className='bg-card flex flex-col justify-between rounded-xl border p-7 shadow-sm'>
            <div>
              <p className='text-primary text-sm font-semibold'>
                {text.outside}
              </p>
              <h3 className='mt-3 text-2xl font-semibold'>
                {text.sourceTitle}
              </h3>
              <p className='text-muted-foreground mt-4'>{text.sourceCopy}</p>
            </div>
            <div className='mt-7 flex flex-wrap gap-3'>
              <Button render={<a {...external(releaseUrl)} />}>
                {text.release}
                <ExternalLink className='ml-2 size-4' />
              </Button>
              <Button
                variant='outline'
                render={<a {...external(requirementsUrl)} />}
              >
                {text.requirements}
              </Button>
            </div>
          </article>
          <article className='bg-card flex flex-col justify-between rounded-xl border p-7 shadow-sm'>
            <div>
              <p className='text-primary text-sm font-semibold'>
                {text.mainland}
              </p>
              <h3 className='mt-3 text-2xl font-semibold'>{text.lazyTitle}</h3>
              <p className='text-muted-foreground mt-4'>{text.lazyCopy}</p>
            </div>
            <div className='mt-7 flex flex-wrap gap-3'>
              <Button render={<a {...external(quarkUrl)} />}>
                {text.quark}
                <ExternalLink className='ml-2 size-4' />
              </Button>
              <Button variant='outline' render={<a {...external(baiduUrl)} />}>
                {text.baidu}
              </Button>
              <span className='text-muted-foreground w-full text-xs'>
                {text.codes}
              </span>
            </div>
          </article>
        </div>
      </section>
      <section id='guide' className='bg-muted/20 border-y px-6 py-20'>
        <div className='mx-auto grid max-w-6xl items-center gap-10 lg:grid-cols-[1fr_.9fr]'>
          <div>
            <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
              Guide / 教程
            </p>
            <h2 className='text-3xl font-semibold md:text-4xl'>
              {text.guideTitle}
            </h2>
            <p className='text-muted-foreground mt-4 max-w-2xl'>
              {text.guideCopy}
            </p>
            <Button className='mt-7' render={<a {...external(guideUrl)} />}>
              {text.webGuide}
              <ArrowRight className='ml-2 size-4' />
            </Button>
          </div>
          <a
            {...external(bilibiliGuideUrl)}
            className='group bg-card rounded-xl border p-3 shadow-sm'
          >
            <img
              src='/lc700x_videoguide.png'
              alt='LC700X Desktop2Stereo Bilibili video guide'
              loading='lazy'
              className='aspect-video w-full rounded-lg object-cover transition-opacity group-hover:opacity-80'
            />
            <span className='mt-4 flex items-center justify-between px-2 pb-2 font-medium'>
              {text.video}
              <ExternalLink className='text-muted-foreground size-4' />
            </span>
          </a>
        </div>
      </section>
      <section id='workflow' className='mx-auto max-w-6xl px-6 py-20'>
        <div className='mb-10 max-w-2xl'>
          <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
            {text.workflow}
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            {text.workflowTitle}
          </h2>
        </div>
        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
          {text.steps.map(([title, description], index) => (
            <article key={title} className='bg-card rounded-xl border p-6'>
              <span className='text-primary text-sm font-bold'>
                0{index + 1}
              </span>
              <h3 className='mt-8 text-xl font-medium'>{title}</h3>
              <p className='text-muted-foreground mt-3 text-sm leading-6'>
                {description}
              </p>
            </article>
          ))}
        </div>
      </section>
      <section id='platforms' className='bg-primary/5 border-y px-6 py-20'>
        <div className='mx-auto max-w-6xl'>
          <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
            {text.platforms}
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            {text.platformsTitle}
          </h2>
          <p className='text-muted-foreground mt-4 max-w-2xl'>
            {text.platformsCopy}
          </p>
          <div className='mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
            {text.platformItems.map(([title, description]) => (
              <article
                key={title}
                className='bg-background/80 rounded-xl border p-5'
              >
                <h3 className='font-medium'>{title}</h3>
                <p className='text-muted-foreground mt-2 text-sm'>
                  {description}
                </p>
              </article>
            ))}
          </div>
          <Button
            variant='outline'
            className='mt-7'
            render={<a {...external(repoUrl)} />}
          >
            <IconGithub className='mr-2 size-4' />
            {text.openGithub}
          </Button>
        </div>
      </section>
      <section
        id='community'
        className='mx-auto grid max-w-6xl items-center gap-10 px-6 py-20 md:grid-cols-[1fr_280px]'
      >
        <div>
          <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
            {text.community}
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            {text.communityTitle}
          </h2>
          <p className='text-muted-foreground mt-4 max-w-2xl'>
            {text.communityCopy}
          </p>
        </div>
        <button
          type='button'
          onClick={() => setLightbox('qq')}
          className='bg-card rounded-xl border p-4 text-left shadow-sm'
        >
          <img
            src='/d2s_qq.jpg'
            alt='D2S official QQ discussion group'
            loading='lazy'
            className='aspect-square w-full rounded-lg object-contain'
          />
          <span className='text-muted-foreground mt-3 flex items-center justify-center gap-2 text-sm'>
            <QrCode className='size-4' />
            {text.enlarge}
          </span>
        </button>
      </section>
      <section id='support' className='bg-muted/20 border-y px-6 py-20'>
        <div className='mx-auto grid max-w-6xl items-center gap-10 md:grid-cols-[1fr_280px]'>
          <div>
            <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
              {text.support}
            </p>
            <h2 className='text-3xl font-semibold md:text-4xl'>
              {text.supportTitle}
            </h2>
            <p className='text-muted-foreground mt-4 max-w-2xl'>
              {text.supportCopy}
            </p>
            <p className='text-primary mt-4 font-medium'>{text.supportNote}</p>
          </div>
          <button
            type='button'
            onClick={() => setLightbox('support')}
            className='bg-card rounded-xl border p-4 text-left shadow-sm'
          >
            <img
              src='/weixin_dashang.png'
              alt='WeChat reward QR code for Desktop2Stereo sponsorship'
              loading='lazy'
              className='aspect-square w-full rounded-lg object-contain'
            />
            <span className='text-muted-foreground mt-3 flex items-center justify-center gap-2 text-sm'>
              <QrCode className='size-4' />
              {text.enlarge}
            </span>
          </button>
        </div>
      </section>
      <section className='mx-auto max-w-6xl px-6 py-16'>
        <div className='bg-card rounded-xl border p-6 shadow-sm'>
          <h2 className='text-2xl font-semibold'>{text.project}</h2>
          <div className='text-muted-foreground mt-5 grid gap-3 text-sm sm:grid-cols-3'>
            {text.projectItems.map((item) => (
              <p key={item} className='flex gap-2'>
                <ImageIcon className='text-primary mt-0.5 size-4 shrink-0' />
                {item}
              </p>
            ))}
          </div>
        </div>
      </section>
      {lightbox ? (
        <div
          role='dialog'
          aria-modal='true'
          aria-label={text.enlarge}
          className='fixed inset-0 z-50 grid place-items-center bg-black/70 p-6'
          onClick={() => setLightbox(null)}
        >
          <div
            className='bg-background relative w-full max-w-lg rounded-2xl p-5 shadow-2xl'
            onClick={(event) => event.stopPropagation()}
          >
            <Button
              variant='ghost'
              size='icon'
              className='absolute top-3 right-3'
              onClick={() => setLightbox(null)}
              aria-label={text.close}
            >
              <X className='size-5' />
            </Button>
            <img
              src={lightbox === 'qq' ? '/d2s_qq.jpg' : '/weixin_dashang.png'}
              alt={text.enlarge}
              className='mx-auto max-h-[75vh] w-full object-contain'
            />
          </div>
        </div>
      ) : null}
    </main>
  )
}
