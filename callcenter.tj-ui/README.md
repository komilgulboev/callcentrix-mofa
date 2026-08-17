<<<<<<< HEAD
<<<<<<< HEAD
# CoreUI Free React Admin Template [![Tweet](https://img.shields.io/twitter/url/http/shields.io.svg?style=social&logo=twitter)](https://twitter.com/intent/tweet?text=CoreUI%20-%20Free%React%204%20Admin%20Template%20&url=https://coreui.io&hashtags=bootstrap,admin,template,dashboard,panel,free,angular,react,vue)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](https://opensource.org/licenses/MIT)
[![@coreui coreui](https://img.shields.io/badge/@coreui%20-coreui-lightgrey.svg?style=flat-square)](https://github.com/coreui/coreui)
[![npm package][npm-coreui-badge]][npm-coreui]
[![NPM downloads][npm-coreui-download]][npm-coreui]
[![@coreui react](https://img.shields.io/badge/@coreui%20-react-lightgrey.svg?style=flat-square)](https://github.com/coreui/react)
[![npm package][npm-coreui-react-badge]][npm-coreui-react]
[![NPM downloads][npm-coreui-react-download]][npm-coreui-react]  

[npm-coreui]: https://www.npmjs.com/package/@coreui/coreui
[npm-coreui-badge]: https://img.shields.io/npm/v/@coreui/coreui.png?style=flat-square
[npm-coreui-download]: https://img.shields.io/npm/dm/@coreui/coreui.svg?style=flat-square
[npm-coreui-react]: https://www.npmjs.com/package/@coreui/react
[npm-coreui-react-badge]: https://img.shields.io/npm/v/@coreui/react.png?style=flat-square
[npm-coreui-react-download]: https://img.shields.io/npm/dm/@coreui/react.svg?style=flat-square
[npm]: https://www.npmjs.com/package/@coreui/react

[![Bootstrap Admin Template](https://assets.coreui.io/products/coreui-free-bootstrap-admin-template-light-dark.webp)](https://coreui.io/product/free-react-admin-template/)

CoreUI is meant to be the UX game changer. Pure & transparent code is devoid of redundant components, so the app is light enough to offer ultimate user experience. This means mobile devices also, where the navigation is just as easy and intuitive as on a desktop or laptop. The CoreUI Layout API lets you customize your project for almost any device – be it Mobile, Web or WebApp – CoreUI covers them all!

## Table of Contents

* [Versions](#versions)
* [CoreUI PRO](#coreui-pro)
* [CoreUI PRO React Admin Templates](#coreui-pro-react-admin-templates)
* [Quick Start](#quick-start)
* [Installation](#installation)
* [Basic usage](#basic-usage)
* [What's included](#whats-included)
* [Documentation](#documentation)
* [Components](#components)
* [Versioning](#versioning)
* [Creators](#creators)
* [Community](#community)
* [Support CoreUI Development](#support-coreui-development)
* [Copyright and License](#copyright-and-license)

## Versions

* [CoreUI Free Bootstrap Admin Template](https://github.com/coreui/coreui-free-bootstrap-admin-template)
* [CoreUI Free Angular Admin Template](https://github.com/coreui/coreui-free-angular-admin-template)
* [CoreUI Free React.js Admin Template (Vite)](https://github.com/coreui/coreui-free-react-admin-template)
* [CoreUI Free React.js Admin Template (Create React App)](https://github.com/coreui/coreui-free-react-admin-template-cra)
* [CoreUI Free Vue.js Admin Template](https://github.com/coreui/coreui-free-vue-admin-template)

## CoreUI PRO

* 💪  [CoreUI PRO Angular Admin Template](https://coreui.io/product/angular-dashboard-template/)
* 💪  [CoreUI PRO Bootstrap Admin Template](https://coreui.io/product/bootstrap-dashboard-template/)
* 💪  [CoreUI PRO Next.js Admin Template](https://coreui.io/product/next-js-dashboard-template/)
* 💪  [CoreUI PRO React Admin Template](https://coreui.io/product/react-dashboard-template/)
* 💪  [CoreUI PRO Vue Admin Template](https://coreui.io/product/vue-dashboard-template/)

## CoreUI PRO React Admin Templates

| Default Theme | Light Theme |
| --- | --- |
| [![CoreUI PRO React Admin Template](https://coreui.io/images/templates/coreui_pro_default_light_dark.webp)](https://coreui.io/product/react-dashboard-template/?theme=default) | [![CoreUI PRO React Admin Template](https://coreui.io/images/templates/coreui_pro_light_light_dark.webp)](https://coreui.io/product/react-dashboard-template/?theme=light)|

| Modern Theme | Bright Theme |
| --- | --- |
| [![CoreUI PRO React Admin Template](https://coreui.io/images/templates/coreui_pro_default_v3_light_dark.webp)](https://coreui.io/product/react-dashboard-template/?theme=modern) | [![CoreUI PRO React Admin Template](https://coreui.io/images/templates/coreui_pro_light_v3_light_dark.webp)](https://coreui.io/product/react-dashboard-template/?theme=bright)|

## Quick Start

- [Download the latest release](https://github.com/coreui/coreui-free-react-admin-template/archive/refs/heads/main.zip)
- Clone the repo: `git clone https://github.com/coreui/coreui-free-react-admin-template.git`

### Installation

``` bash
$ npm install
```

or

``` bash
$ yarn install
```

### Basic usage

``` bash
# dev server with hot reload at http://localhost:3000
$ npm start 
```

or 

``` bash
# dev server with hot reload at http://localhost:3000
$ yarn start
```

Navigate to [http://localhost:3000](http://localhost:3000). The app will automatically reload if you change any of the source files.

#### Build

Run `build` to build the project. The build artifacts will be stored in the `build/` directory.

```bash
# build for production with minification
$ npm run build
```

or

```bash
# build for production with minification
$ yarn build
```

## What's included

Within the download you'll find the following directories and files, logically grouping common assets and providing both compiled and minified variations. You'll see something like this:

```
coreui-free-react-admin-template
├── public/          # static files
│   ├── favicon.ico
│   └── manifest.json
│
├── src/             # project root
│   ├── assets/      # images, icons, etc.
│   ├── components/  # common components - header, footer, sidebar, etc.
│   ├── layouts/     # layout containers
│   ├── scss/        # scss styles
│   ├── views/       # application views
│   ├── _nav.js      # sidebar navigation config
│   ├── App.js
│   ├── index.js
│   ├── routes.js    # routes config
│   └── store.js     # template state example 
│
├── index.html       # html template
├── ...
├── package.json
├── ...
└── vite.config.mjs  # vite config
```

## Documentation

The documentation for the CoreUI Admin Template is hosted at our website [CoreUI for React](https://coreui.io/react/docs/templates/installation/)

## Components

CoreUI React.js Admin Templates are built on top of CoreUI and CoreUI PRO UI components libraries, including all of these components.

- [React Accordion](https://coreui.io/react/docs/components/accordion/)
- [React Alert](https://coreui.io/react/docs/components/alert/)
- [React Autocomplete](https://coreui.io/react/docs/forms/autocomplete/) **PRO**
- [React Avatar](https://coreui.io/react/docs/components/avatar/)
- [React Badge](https://coreui.io/react/docs/components/badge/)
- [React Breadcrumb](https://coreui.io/react/docs/components/breadcrumb/)
- [React Button](https://coreui.io/react/docs/components/button/)
- [React Button Group](https://coreui.io/react/docs/components/button-group/)
- [React Callout](https://coreui.io/react/docs/components/callout/)
- [React Card](https://coreui.io/react/docs/components/card/)
- [React Carousel](https://coreui.io/react/docs/components/carousel/)
- [React Checkbox](https://coreui.io/react/docs/forms/checkbox/)
- [React Close Button](https://coreui.io/react/docs/components/close-button/)
- [React Collapse](https://coreui.io/react/docs/components/collapse/)
- [React Date Picker](https://coreui.io/react/docs/forms/date-picker/) **PRO**
- [React Date Range Picker](https://coreui.io/react/docs/forms/date-range-picker/) **PRO**
- [React Dropdown](https://coreui.io/react/docs/components/dropdown/)
- [React Floating Labels](https://coreui.io/react/docs/forms/floating-labels/)
- [React Footer](https://coreui.io/react/docs/components/footer/)
- [React Header](https://coreui.io/react/docs/components/header/)
- [React Image](https://coreui.io/react/docs/components/image/)
- [React Input](https://coreui.io/react/docs/forms/input/)
- [React Input Group](https://coreui.io/react/docs/forms/input-group/)
- [React List Group](https://coreui.io/react/docs/components/list-group/)
- [React Loading Button](https://coreui.io/react/docs/components/loading-button/) **PRO**
- [React Modal](https://coreui.io/react/docs/components/modal/)
- [React Multi Select](https://coreui.io/react/docs/forms/multi-select/) **PRO**
- [React Navs & Tabs](https://coreui.io/react/docs/components/navs-tabs/)
- [React Navbar](https://coreui.io/react/docs/components/navbar/)
- [React Offcanvas](https://coreui.io/react/docs/components/offcanvas/)
- [React One Time Password Input](https://coreui.io/react/docs/forms/one-time-password-input/) **PRO**
- [React Pagination](https://coreui.io/react/docs/components/pagination/)
- [React Password Input](https://coreui.io/react/docs/forms/password-input/) **PRO**
- [React Placeholder](https://coreui.io/react/docs/components/placeholder/)
- [React Popover](https://coreui.io/react/docs/components/popover/)
- [React Progress](https://coreui.io/react/docs/components/progress/)
- [React Radio](https://coreui.io/react/docs/forms/radio/)
- [React Range](https://coreui.io/react/docs/forms/range/)
- [React Range Slider](https://coreui.io/react/docs/forms/range-slider/) **PRO**
- [React Rating](https://coreui.io/react/docs/forms/rating/)
- [React Select](https://coreui.io/react/docs/forms/select/)
- [React Sidebar](https://coreui.io/react/docs/components/sidebar/)
- [React Smart Pagination](https://coreui.io/react/docs/components/smart-pagination/) **PRO**
- [React Smart Table](https://coreui.io/react/docs/components/smart-table/) **PRO**
- [React Spinner](https://coreui.io/react/docs/components/spinner/)
- [React Stepper](https://coreui.io/react/docs/forms/stepper/) **PRO**
- [React Switch](https://coreui.io/react/docs/forms/switch/)
- [React Table](https://coreui.io/react/docs/components/table/)
- [React Textarea](https://coreui.io/react/docs/forms/textarea/)
- [React Time Picker](https://coreui.io/react/docs/forms/time-picker/) **PRO**
- [React Toast](https://coreui.io/react/docs/components/toast/)
- [React Tooltip](https://coreui.io/react/docs/components/tooltip/)

## Versioning

For transparency into our release cycle and in striving to maintain backward compatibility, CoreUI Free Admin Template is maintained under [the Semantic Versioning guidelines](http://semver.org/).

See [the Releases section of our project](https://github.com/coreui/coreui-free-react-admin-template/releases) for changelogs for each release version.

## Creators

**Łukasz Holeczek**

* <https://twitter.com/lukaszholeczek>
* <https://github.com/mrholek>

**Andrzej Kopański**

* <https://github.com/xidedix>

**CoreUI Team**

* <https://twitter.com/core_ui>
* <https://github.com/coreui>
* <https://github.com/orgs/coreui/people>

## Community

Get updates on CoreUI's development and chat with the project maintainers and community members.

- Follow [@core_ui on Twitter](https://twitter.com/core_ui).
- Read and subscribe to [CoreUI Blog](https://coreui.ui/blog/).

## Support CoreUI Development

CoreUI is an MIT-licensed open source project and is completely free to use. However, the amount of effort needed to maintain and develop new features for the project is not sustainable without proper financial backing. You can support development by buying the [CoreUI PRO](https://coreui.io/pricing/?framework=react&src=github-coreui-free-react-admin-template) or by becoming a sponsor via [Open Collective](https://opencollective.com/coreui/).

## Copyright and License

copyright 2025 creativeLabs Łukasz Holeczek.   

Code released under [the MIT license](https://github.com/coreui/coreui-free-react-admin-template/blob/main/LICENSE).
=======
<<<<<<< HEAD
=======
>>>>>>> a42128df7d8bcc574702a095279c5651441b4c51
# callcenter.tj-ui



## Getting started

To make it easy for you to get started with GitLab, here's a list of recommended next steps.

Already a pro? Just edit this README.md and make it your own. Want to make it easy? [Use the template at the bottom](#editing-this-readme)!

## Add your files

* [Create](https://docs.gitlab.com/ee/user/project/repository/web_editor.html#create-a-file) or [upload](https://docs.gitlab.com/ee/user/project/repository/web_editor.html#upload-a-file) files
* [Add files using the command line](https://docs.gitlab.com/topics/git/add_files/#add-files-to-a-git-repository) or push an existing Git repository with the following command:

```
cd existing_repo
git remote add origin https://gitlab.com/komil.gulboev/callcenter.tj-ui.git
git branch -M main
git push -uf origin main
```

## Integrate with your tools

* [Set up project integrations](https://gitlab.com/komil.gulboev/callcenter.tj-ui/-/settings/integrations)

## Collaborate with your team

* [Invite team members and collaborators](https://docs.gitlab.com/ee/user/project/members/)
* [Create a new merge request](https://docs.gitlab.com/ee/user/project/merge_requests/creating_merge_requests.html)
* [Automatically close issues from merge requests](https://docs.gitlab.com/ee/user/project/issues/managing_issues.html#closing-issues-automatically)
* [Enable merge request approvals](https://docs.gitlab.com/ee/user/project/merge_requests/approvals/)
* [Set auto-merge](https://docs.gitlab.com/user/project/merge_requests/auto_merge/)

## Test and Deploy

Use the built-in continuous integration in GitLab.

* [Get started with GitLab CI/CD](https://docs.gitlab.com/ee/ci/quick_start/)
* [Analyze your code for known vulnerabilities with Static Application Security Testing (SAST)](https://docs.gitlab.com/ee/user/application_security/sast/)
* [Deploy to Kubernetes, Amazon EC2, or Amazon ECS using Auto Deploy](https://docs.gitlab.com/ee/topics/autodevops/requirements.html)
* [Use pull-based deployments for improved Kubernetes management](https://docs.gitlab.com/ee/user/clusters/agent/)
* [Set up protected environments](https://docs.gitlab.com/ee/ci/environments/protected_environments.html)

***

# Editing this README

When you're ready to make this README your own, just edit this file and use the handy template below (or feel free to structure it however you want - this is just a starting point!). Thanks to [makeareadme.com](https://www.makeareadme.com/) for this template.

## Suggestions for a good README

Every project is different, so consider which of these sections apply to yours. The sections used in the template are suggestions for most open source projects. Also keep in mind that while a README can be too long and detailed, too long is better than too short. If you think your README is too long, consider utilizing another form of documentation rather than cutting out information.

## Name
Choose a self-explaining name for your project.

## Description
Let people know what your project can do specifically. Provide context and add a link to any reference visitors might be unfamiliar with. A list of Features or a Background subsection can also be added here. If there are alternatives to your project, this is a good place to list differentiating factors.

## Badges
On some READMEs, you may see small images that convey metadata, such as whether or not all the tests are passing for the project. You can use Shields to add some to your README. Many services also have instructions for adding a badge.

## Visuals
Depending on what you are making, it can be a good idea to include screenshots or even a video (you'll frequently see GIFs rather than actual videos). Tools like ttygif can help, but check out Asciinema for a more sophisticated method.

## Installation
Within a particular ecosystem, there may be a common way of installing things, such as using Yarn, NuGet, or Homebrew. However, consider the possibility that whoever is reading your README is a novice and would like more guidance. Listing specific steps helps remove ambiguity and gets people to using your project as quickly as possible. If it only runs in a specific context like a particular programming language version or operating system or has dependencies that have to be installed manually, also add a Requirements subsection.

## Usage
Use examples liberally, and show the expected output if you can. It's helpful to have inline the smallest example of usage that you can demonstrate, while providing links to more sophisticated examples if they are too long to reasonably include in the README.

## Support
Tell people where they can go to for help. It can be any combination of an issue tracker, a chat room, an email address, etc.

## Roadmap
If you have ideas for releases in the future, it is a good idea to list them in the README.

## Contributing
State if you are open to contributions and what your requirements are for accepting them.

For people who want to make changes to your project, it's helpful to have some documentation on how to get started. Perhaps there is a script that they should run or some environment variables that they need to set. Make these steps explicit. These instructions could also be useful to your future self.

You can also document commands to lint the code or run tests. These steps help to ensure high code quality and reduce the likelihood that the changes inadvertently break something. Having instructions for running tests is especially helpful if it requires external setup, such as starting a Selenium server for testing in a browser.

## Authors and acknowledgment
Show your appreciation to those who have contributed to the project.

## License
For open source projects, say how it is licensed.

## Project status
If you have run out of energy or time for your project, put a note at the top of the README saying that development has slowed down or stopped completely. Someone may choose to fork your project or volunteer to step in as a maintainer or owner, allowing your project to keep going. You can also make an explicit request for maintainers.
=======
### Check out our React Admin Templates and support CoreUI Development

[<img src="https://genesisui.com/img/bundle2.png">](https://genesisui.com/bundle.html?support=1&utm_source=github&utm_medium=referer&utm_campaign=angular_version)

[Check out React Admin Templates Bundle](https://genesisui.com/bundle.html?support=1&utm_source=github&utm_medium=referer&utm_campaign=angular_version)

# CoreUI React Version - Free React Admin Template

This is React.js version of our Bootstrap 4 admin template [CoreUI](https://github.com/mrholek/CoreUI-Free-Bootstrap-Admin-Template).

Please help us on [Product Hunt](https://www.producthunt.com/posts/coreui-open-source-bootstrap-4-admin-template-with-angular-2-react-js-vue-js-support) & [Designer News](https://www.designernews.co/stories/81127). Thanks in advance!

Why I decided to create CoreUI? Please read this article: [Jack of all trades, master of none. Why Boostrap Admin Templates sucks.](https://medium.com/@lukaszholeczek/jack-of-all-trades-master-of-none-5ea53ef8a1f#.7eqx1bcd8)

CoreUI is Open Source React & Bootstrap Admin Template. CoreUI is not just another Admin Template. It goes way beyond hitherto admin templates thanks to transparent code and file structure. And if it’s not enough, let’s just add the CoreUI consists bunch of unique features and over 1000 high quality icons.,

CoreUI based on Bootstrap 4 and offers 6 versions: HTML5, AJAX, AngularJS, Angular 2, React.js & Vue.js.

CoreUI is meant to be the UX game changer. Pure & transparent code is devoid of redundant components, so the app is light enough to offer ultimate user experience. This means also mobile devices, where the navigation is the same easy and intuitive as in desktop or laptop. CoreUI Layout API lets you customize your project for almost any device – be it Mobile, Web or WebApp – CoreUI covers them all!

<img src="http://coreui.io/assets/img/coreui.png" alt="Free Angular Admin Template">

**NOTE:** Please remember to **STAR** this project and **FOLLOW** [my Github](https://github.com/mrholek) to keep you update with this template.

## Demo

A fully functional demo is available at <a href="http://coreui.io/examples">CoreUI</a>

## What's included

Within the download you'll find the following directories and files, You'll see something like this:

```
CoreUI-React/
├── React_Full_Project/
├── React_CLI_Starter/

```

## Other Versions

CoreUI includes 6 Version for Angular 4, AngularJS, React.js, Vue.js, Static HTML5 and AJAX HTML5.

* [Angular Version (Angular 2+)](https://github.com/mrholek/CoreUI-Angular).
* [AngularJS Version](https://github.com/mrholek/CoreUI-AngularJS).
* [HTML5 AJAX Version](https://github.com/mrholek/CoreUI-Free-Bootstrap-Admin-Template).
* [HTML5 Static Version](https://github.com/mrholek/CoreUI-Free-Bootstrap-Admin-Template).
* [HTML5 React.js Version](https://github.com/mrholek/CoreUI-React).
* [HTML5 Vue.js Version](https://github.com/mrholek/CoreUI-Vue).

## Bugs and feature requests

Have a bug or a feature request? [Please open a new issue](https://github.com/mrholek/CoreUI-React/issues/new).

## Documentation

CoreUI's documentation, is hosted on our website <a href="http://coreui.io">coreui.io</a>


## Copyright and license

copyright 2017 creativeLabs Łukasz Holeczek. Code released under [the MIT license](https://github.com/mrholek/CoreUI-React/blob/master/LICENSE).
creativeLabs Łukasz Holeczek reserves the right to change the license of future releases. You can’t re-distribute the CoreUI as stock. You can’t do this if you modify the CoreUI.

## Support CoreUI Development

CoreUI is an MIT licensed open source project and completely free to use. However, the amount of effort needed to maintain and develop new features for the project is not sustainable without proper financial backing. You can support development by donating on [PayPal](https://www.paypal.me/holeczek) or buying one of our [premium bootstrap 4 admin templates](https://genesisui.com/bundle.html?support=1&utm_source=github&utm_medium=referer&utm_campaign=react_version).

As of now I am exploring the possibility of working on CoreUI fulltime - if you are a business that is building core products using CoreUI, I am also open to conversations regarding custom sponsorship / consulting arrangements. Get in touch on [Twitter](https://twitter.com/lukaszholeczek).
>>>>>>> c3354fa (Hello World!)
<<<<<<< HEAD
>>>>>>> a42128df7d8bcc574702a095279c5651441b4c51
=======
>>>>>>> a42128df7d8bcc574702a095279c5651441b4c51
