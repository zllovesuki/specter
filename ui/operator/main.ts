import { mount } from 'svelte';
import '../shared/theme.css';
import App from './App.svelte';

mount(App, { target: document.getElementById('app')! });
