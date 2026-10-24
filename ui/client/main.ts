import { mount } from "svelte";
import App from "./App.svelte";
import Root from "../shared/Root.svelte";
import "../shared/theme.css";

mount(Root, { target: document.getElementById("app")!, props: { app: App } });
