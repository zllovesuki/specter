import { mount } from 'svelte';
import '../shared/theme.css';
import App from './App.svelte';
import Root from '../shared/Root.svelte';

mount(Root, { target: document.getElementById('app')!, props: { app: App } });
