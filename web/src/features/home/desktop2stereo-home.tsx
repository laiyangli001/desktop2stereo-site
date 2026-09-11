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

const features = [
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
]

const parameterGroups = [
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
]

const workflow = [
  [
    '01',
    'Download',
    'Pick the GitHub source or China lazy package for your region and setup preference.',
  ],
  [
    '02',
    'Install',
    'Create the Python environment and install requirements.txt as described in the README.',
  ],
  [
    '03',
    'Run',
    'Follow the quick-run instructions and choose the proper mode for desktop, video, or game content.',
  ],
  [
    '04',
    'Tune',
    'Adjust stereo strength, depth model, display mode, and runtime settings for your hardware.',
  ],
]

const external = (href: string) => ({
  href,
  target: '_blank',
  rel: 'noreferrer',
})

export function Desktop2StereoHome({
  isAuthenticated,
}: Desktop2StereoHomeProps) {
  const [lightbox, setLightbox] = useState<'qq' | 'support' | null>(null)

  return (
    <main className='d2s-home bg-background text-foreground'>
      <section className='relative isolate overflow-hidden border-b px-6 py-20 md:py-28'>
        <img
          src='/hero-stereo-lab.png'
          alt='Desktop2Steoro stereo desktop workstation'
          className='absolute inset-0 -z-20 h-full w-full object-cover object-center opacity-25 dark:opacity-35'
        />
        <div className='from-background via-background/95 to-background/60 absolute inset-0 -z-10 bg-gradient-to-r' />
        <div className='mx-auto grid max-w-6xl items-center gap-12 lg:grid-cols-[1.05fr_.95fr]'>
          <div>
            <p className='text-primary mb-4 text-sm font-semibold tracking-[.18em] uppercase'>
              Official Desktop2Stereo Website
            </p>
            <h1 className='max-w-3xl text-4xl font-bold tracking-tight md:text-6xl'>
              Desktop2Steoro <span className='text-primary'>v2.5</span>
            </h1>
            <p className='text-muted-foreground mt-6 max-w-2xl text-lg leading-8'>
              Convert ordinary desktop, video, game, and media content into
              stereo 3D output for large displays, VR viewing, and 3D display
              experiments.
            </p>
            <div className='mt-8 flex flex-wrap gap-3'>
              <Button render={<a {...external(releaseUrl)} />}>
                <Download className='mr-2 size-4' />
                Download v2.5
              </Button>
              <Button variant='outline' render={<a {...external(guideUrl)} />}>
                <PlayCircle className='mr-2 size-4' />
                Read install guide
              </Button>
              {isAuthenticated ? (
                <Button variant='ghost' render={<Link to='/dashboard' />}>
                  Go to Dashboard
                  <ArrowRight className='ml-2 size-4' />
                </Button>
              ) : null}
            </div>
            <div className='text-muted-foreground mt-8 flex flex-wrap gap-2 text-sm'>
              {[
                'Official GitHub project',
                'Source and lazy packages',
                'OpenXR ready',
                'Chinese and English resources',
              ].map((item) => (
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
            <img
              src='/hero-stereo-lab.png'
              alt='Desktop2Steoro depth and stereo preview'
              className='aspect-[4/3] w-full rounded-xl object-cover'
            />
          </div>
        </div>
      </section>

      <section className='border-b px-6 py-10'>
        <div className='mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4'>
          <p className='text-muted-foreground max-w-xl text-lg'>
            Cross-vendor hardware, multiple operating systems, and several
            output paths for real 2D-to-3D workflows.
          </p>
          <div className='flex max-w-2xl flex-wrap justify-end gap-2 text-sm'>
            {[
              'NVIDIA / AMD / Intel',
              'Apple Silicon',
              'CUDA / ROCm',
              'GPU zero-copy',
              'OpenXR',
              'Network streaming',
            ].map((item) => (
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
            Compatibility
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            Supported hardware and operating systems
          </h2>
          <p className='text-muted-foreground mt-4'>
            Desktop2Steoro is designed for cross-vendor GPU acceleration and
            mainstream desktop operating systems.
          </p>
        </div>
        <div className='grid gap-4 md:grid-cols-2'>
          {[
            [
              'Hardware',
              [
                'NVIDIA GPU',
                'AMD GPU',
                'Intel GPU',
                'Apple Silicon M1–M4',
                'DirectML devices on Windows',
              ],
            ],
            [
              'Operating systems',
              [
                'Windows 10/11 (x64 / Arm64)',
                'macOS 10.16 or later',
                'Ubuntu 22.04 or later',
              ],
            ],
          ].map(([title, items]) => (
            <article
              key={title as string}
              className='bg-card rounded-xl border p-6 shadow-sm'
            >
              <h3 className='mb-4 flex items-center gap-2 text-xl font-medium'>
                <Monitor className='text-primary size-5' />
                {title as string}
              </h3>
              <ul className='text-muted-foreground space-y-3'>
                {(items as string[]).map((item) => (
                  <li key={item} className='flex gap-2'>
                    <Check className='text-primary mt-0.5 size-4 shrink-0' />
                    {item}
                  </li>
                ))}
              </ul>
            </article>
          ))}
        </div>
      </section>

      <section id='capabilities' className='bg-muted/20 border-y px-6 py-20'>
        <div className='mx-auto max-w-6xl'>
          <div className='mb-10 max-w-3xl'>
            <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
              Features
            </p>
            <h2 className='text-3xl font-semibold md:text-4xl'>
              Broad hardware support, tunable stereo parameters, multiple 3D
              outputs.
            </h2>
            <p className='text-muted-foreground mt-4'>
              A layered real-time pipeline captures input, estimates depth,
              synthesizes stereo eyes, and presents the result to local display,
              OpenXR, stream, export, or API targets.
            </p>
          </div>
          <div className='grid gap-4 md:grid-cols-2 lg:grid-cols-3'>
            {features.map(([title, description]) => (
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
            {parameterGroups.map(([title, description]) => (
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
            Download
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            Two download paths
          </h2>
          <p className='text-muted-foreground mt-4'>
            Mainland China users should usually start with the lazy package.
            Other users can start from GitHub source and install Python
            requirements manually.
          </p>
        </div>
        <div className='grid gap-4 md:grid-cols-2'>
          <article className='bg-card flex flex-col justify-between rounded-xl border p-7 shadow-sm'>
            <div>
              <p className='text-primary text-sm font-semibold'>
                Outside mainland China
              </p>
              <h3 className='mt-3 text-2xl font-semibold'>
                Source package from GitHub
              </h3>
              <p className='text-muted-foreground mt-4'>
                Download the repository source with requirements.txt, then
                create and install the Python runtime environment yourself.
              </p>
            </div>
            <div className='mt-7 flex flex-wrap gap-3'>
              <Button render={<a {...external(releaseUrl)} />}>
                Open v2.5.0 releases
                <ExternalLink className='ml-2 size-4' />
              </Button>
              <Button
                variant='outline'
                render={<a {...external(requirementsUrl)} />}
              >
                requirements.txt
              </Button>
            </div>
          </article>
          <article className='bg-card flex flex-col justify-between rounded-xl border p-7 shadow-sm'>
            <div>
              <p className='text-primary text-sm font-semibold'>
                Mainland China
              </p>
              <h3 className='mt-3 text-2xl font-semibold'>
                Lazy package with Python runtime
              </h3>
              <p className='text-muted-foreground mt-4'>
                The prepackaged archive includes a complete Python runtime
                environment to reduce setup friction.
              </p>
            </div>
            <div className='mt-7 flex flex-wrap gap-3'>
              <Button render={<a {...external(quarkUrl)} />}>
                Quark Netdisk
                <ExternalLink className='ml-2 size-4' />
              </Button>
              <Button variant='outline' render={<a {...external(baiduUrl)} />}>
                Baidu Netdisk
              </Button>
              <span className='text-muted-foreground w-full text-xs'>
                Extraction codes: Quark 1vcn · Baidu mr64
              </span>
            </div>
          </article>
        </div>
      </section>

      <section id='guide' className='bg-muted/20 border-y px-6 py-20'>
        <div className='mx-auto grid max-w-6xl items-center gap-10 lg:grid-cols-[1fr_.9fr]'>
          <div>
            <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
              Guide
            </p>
            <h2 className='text-3xl font-semibold md:text-4xl'>
              Use the official README as the operating manual.
            </h2>
            <p className='text-muted-foreground mt-4 max-w-2xl'>
              The README in lc700x/desktop2stereo is the canonical source for
              hardware requirements, dependency installation, quick start, model
              selection, and troubleshooting.
            </p>
            <Button className='mt-7' render={<a {...external(guideUrl)} />}>
              Open web guide
              <ArrowRight className='ml-2 size-4' />
            </Button>
          </div>
          <a
            {...external(bilibiliGuideUrl)}
            className='group bg-card rounded-xl border p-3 shadow-sm'
          >
            <img
              src='/lc700x_videoguide.png'
              alt='LC700X Desktop2Steoro Bilibili video guide'
              loading='lazy'
              className='aspect-video w-full rounded-lg object-cover transition-opacity group-hover:opacity-80'
            />
            <span className='mt-4 flex items-center justify-between px-2 pb-2 font-medium'>
              Bilibili video tutorials
              <ExternalLink className='text-muted-foreground size-4' />
            </span>
          </a>
        </div>
      </section>

      <section id='workflow' className='mx-auto max-w-6xl px-6 py-20'>
        <div className='mb-10 max-w-2xl'>
          <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
            Workflow
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            From source to stereo output.
          </h2>
        </div>
        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
          {workflow.map(([step, title, description]) => (
            <article key={step} className='bg-card rounded-xl border p-6'>
              <span className='text-primary text-sm font-bold'>{step}</span>
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
            Platforms
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            Version and project links
          </h2>
          <p className='text-muted-foreground mt-4 max-w-2xl'>
            Current website version label: Desktop2Steoro v2.5. The official
            code and tutorial source is lc700x/desktop2stereo.
          </p>
          <div className='mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
            {[
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
              [
                'Manual source install',
                'GitHub ZIP plus Python dependency setup.',
              ],
            ].map(([title, description]) => (
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
            Open official GitHub
          </Button>
        </div>
      </section>

      <section
        id='community'
        className='mx-auto grid max-w-6xl items-center gap-10 px-6 py-20 md:grid-cols-[1fr_280px]'
      >
        <div>
          <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
            Community
          </p>
          <h2 className='text-3xl font-semibold md:text-4xl'>
            Join the official QQ discussion group.
          </h2>
          <p className='text-muted-foreground mt-4 max-w-2xl'>
            For usage questions, troubleshooting, and Chinese community
            discussion, scan the QR code to join the D2S official QQ group.
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
            Enlarge QR code
          </span>
        </button>
      </section>

      <section id='support' className='bg-muted/20 border-y px-6 py-20'>
        <div className='mx-auto grid max-w-6xl items-center gap-10 md:grid-cols-[1fr_280px]'>
          <div>
            <p className='text-primary mb-3 text-sm font-semibold tracking-[.18em] uppercase'>
              Support
            </p>
            <h2 className='text-3xl font-semibold md:text-4xl'>
              Support Desktop2Steoro
            </h2>
            <p className='text-muted-foreground mt-4 max-w-2xl'>
              Turning 2D video into real-time 3D in VR is expensive to maintain.
              Sponsorship helps fund monthly model tokens, distribution storage,
              and continued image-quality work.
            </p>
            <p className='text-primary mt-4 font-medium'>
              If sponsorship is not convenient, a free GitHub Star or sharing
              the project in a VR group helps just as much.
            </p>
          </div>
          <button
            type='button'
            onClick={() => setLightbox('support')}
            className='bg-card rounded-xl border p-4 text-left shadow-sm'
          >
            <img
              src='/weixin_dashang.png'
              alt='WeChat reward QR code for Desktop2Steoro sponsorship'
              loading='lazy'
              className='aspect-square w-full rounded-lg object-contain'
            />
            <span className='text-muted-foreground mt-3 flex items-center justify-center gap-2 text-sm'>
              <QrCode className='size-4' />
              Enlarge QR code
            </span>
          </button>
        </div>
      </section>

      <section className='mx-auto max-w-6xl px-6 py-16'>
        <div className='bg-card rounded-xl border p-6 shadow-sm'>
          <h2 className='text-2xl font-semibold'>Project information</h2>
          <div className='text-muted-foreground mt-5 grid gap-3 text-sm sm:grid-cols-3'>
            {[
              'Current software version: Desktop2Steoro v2.5',
              'Official source: lc700x/desktop2stereo on GitHub',
              'Manual resources: English / 中文',
            ].map((item) => (
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
          aria-label='QR code preview'
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
              aria-label='Close'
            >
              <X className='size-5' />
            </Button>
            <img
              src={lightbox === 'qq' ? '/d2s_qq.jpg' : '/weixin_dashang.png'}
              alt={
                lightbox === 'qq'
                  ? 'D2S official QQ discussion group'
                  : 'Desktop2Steoro sponsorship QR code'
              }
              className='mx-auto max-h-[75vh] w-full object-contain'
            />
          </div>
        </div>
      ) : null}
    </main>
  )
}
