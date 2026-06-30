package dev.powermine.forgeagent;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.google.gson.JsonPrimitive;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import net.minecraftforge.fml.common.FMLCommonHandler;
import net.minecraftforge.fml.common.Mod;
import net.minecraftforge.fml.common.event.FMLInitializationEvent;
import net.minecraftforge.fml.relauncher.Side;

import java.awt.image.BufferedImage;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.lang.reflect.Array;
import java.lang.reflect.Constructor;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.net.InetSocketAddress;
import java.net.URLDecoder;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import javax.imageio.ImageIO;

@Mod(
        modid = PowerMineForgeAgent.MOD_ID,
        name = "Power Mine Forge Agent",
        version = PowerMineForgeAgent.VERSION,
        acceptedMinecraftVersions = "[1.12.2]",
        acceptableRemoteVersions = "*"
)
public final class PowerMineForgeAgent {
    public static final String MOD_ID = "power_mine_agent";
    public static final String VERSION = "0.1.0";

    private static final Gson GSON = new GsonBuilder().disableHtmlEscaping().create();
    private static final int DEFAULT_PORT = 39276;
    private static final int MAX_SNAPSHOT_RADIUS = 8;

    private HttpServer server;

    @Mod.EventHandler
    public void init(FMLInitializationEvent event) {
        if (FMLCommonHandler.instance().getSide() != Side.CLIENT) {
            return;
        }
        int port = configuredPort();
        String token = configuredValue("power.mine.agent.token", "POWER_MINE_AGENT_TOKEN", "");
        try {
            server = HttpServer.create(new InetSocketAddress("127.0.0.1", port), 0);
            register("GET", "/health", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) {
                    return health();
                }
            });
            register("GET", "/bridge/capabilities", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) {
                    return bridgeCapabilities();
                }
            });
            register("GET", "/capabilities", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) {
                    return bridgeCapabilities();
                }
            });
            register("GET", "/state", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return state();
                }
            });
            register("GET", "/inventory", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return inventory();
                }
            });
            register("GET", "/world/snapshot", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return worldSnapshot(exchange);
                }
            });
            register("POST", "/world/open", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return openWorld(parseBody(exchange));
                }
            });
            register("GET", "/render/held-item", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return heldItemRender(exchange);
                }
            });
            register("GET", "/render/block", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return blockRender(exchange);
                }
            });
            register("GET", "/render/screenshot", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return screenshot();
                }
            });
            register("POST", "/input/release", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return releaseInput();
                }
            });
            register("POST", "/inventory/give", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return giveItem(parseBody(exchange));
                }
            });
            register("POST", "/hotbar/select", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return selectHotbar(parseBody(exchange));
                }
            });
            register("POST", "/block/place", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return placeBlock(parseBody(exchange));
                }
            });
            register("POST", "/block/break", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return breakBlock(parseBody(exchange));
                }
            });
            register("POST", "/recipe/check", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return recipeCheck(parseBody(exchange));
                }
            });
            register("POST", "/recipe/craft", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return craftRecipe(parseBody(exchange));
                }
            });
            register("POST", "/tick/wait", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return waitTicks(parseBody(exchange));
                }
            });
            register("POST", "/item/use", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return useItem(parseBody(exchange));
                }
            });
            register("POST", "/block/use", token, new Route() {
                @Override
                public JsonObject handle(HttpExchange exchange) throws Exception {
                    return useBlock(parseBody(exchange));
                }
            });
            server.setExecutor(Executors.newCachedThreadPool());
            server.start();
            disablePauseOnLostFocus();
            System.out.println("[Power Mine Forge Agent] listening on 127.0.0.1:" + port);
        } catch (IOException exc) {
            System.err.println("[Power Mine Forge Agent] failed to start: " + exc.getMessage());
        }
    }

    private void register(String method, String path, String token, Route route) {
        server.createContext(path, exchange -> {
            try {
                if (!method.equals(exchange.getRequestMethod())) {
                    write(exchange, 405, error("method_not_allowed", "Expected " + method));
                    return;
                }
                if (!authorized(exchange, token)) {
                    write(exchange, 401, error("unauthorized", "Invalid Power Mine agent token"));
                    return;
                }
                JsonObject payload = route.handle(exchange);
                int status = payload.has("httpStatus") ? payload.get("httpStatus").getAsInt() : 200;
                write(exchange, status, payload);
            } catch (IllegalArgumentException exc) {
                write(exchange, 400, error("bad_request", exc.getMessage()));
            } catch (UnsupportedOperationException exc) {
                write(exchange, 501, error("unsupported", exc.getMessage()));
            } catch (Exception exc) {
                write(exchange, 500, error("agent_error", exc.getMessage()));
            }
        });
    }

    private Route unsupportedRoute(final String message) {
        return new Route() {
            @Override
            public JsonObject handle(HttpExchange exchange) {
                JsonObject result = error("unsupported", message);
                result.addProperty("httpStatus", 501);
                return result;
            }
        };
    }

    private JsonObject health() {
        JsonObject result = ok();
        result.addProperty("name", "power-mine-forge-agent");
        result.addProperty("version", VERSION);
        result.addProperty("protocolVersion", "power-mine-agent/v1");
        result.addProperty("loader", "forge");
        result.addProperty("minecraftVersion", "1.12.2");
        result.addProperty("minecraftThread", minecraft() != null);
        return result;
    }

    private JsonObject bridgeCapabilities() {
        JsonObject result = ok();
        result.addProperty("protocolVersion", "power-mine-agent/v1");
        result.addProperty("agentId", "power-mine-forge-1.12.2-agent");
        result.addProperty("agentVersion", VERSION);
        result.addProperty("loader", "forge");
        result.addProperty("minecraftVersion", "1.12.2");

        JsonObject capabilities = new JsonObject();
        capabilities.addProperty("state", true);
        capabilities.addProperty("inventory", true);
        capabilities.addProperty("giveItem", true);
        capabilities.addProperty("selectHotbar", true);
        capabilities.addProperty("worldSnapshot", true);
        capabilities.addProperty("screenshot", true);
        capabilities.addProperty("recipeCheck", true);
        capabilities.addProperty("craftRecipe", true);
        capabilities.addProperty("placeBlock", true);
        capabilities.addProperty("breakBlock", true);
        capabilities.addProperty("waitTicks", true);
        capabilities.addProperty("useItem", true);
        capabilities.addProperty("useBlock", true);
        capabilities.addProperty("heldItemRender", true);
        capabilities.addProperty("blockRender", true);
        capabilities.addProperty("bakedModelIntrospection", false);
        capabilities.addProperty("screenshotVisualProbe", true);
        capabilities.addProperty("pauseOnLostFocusControl", true);
        capabilities.addProperty("inputRelease", true);
        capabilities.addProperty("offhand", true);
        capabilities.addProperty("autoWorldOpen", true);
        capabilities.addProperty("autoWorldCreate", true);
        result.add("capabilities", capabilities);

        JsonArray limitations = new JsonArray();
        limitations.add(new JsonPrimitive("Forge 1.12.2 recipe checks report registry ids when available and fall back to the matched recipe class."));
        limitations.add(new JsonPrimitive("Held-item and block render checks use screenshot visual probes, not modern baked-model introspection."));
        limitations.add(new JsonPrimitive("The agent disables pause-on-lost-focus for automation sessions and can release the mouse cursor on request."));
        limitations.add(new JsonPrimitive("Block mutations and auto-created worlds should only be used in local singleplayer test worlds."));
        result.add("limitations", limitations);
        return result;
    }

    private JsonObject state() throws Exception {
        Object mc = requireMinecraft();
        Object player = field(mc, "player", "thePlayer", "field_71439_g");
        Object world = field(mc, "world", "theWorld", "field_71441_e");
        JsonObject result = ok();
        result.addProperty("loaded", player != null && world != null);
        result.addProperty("singleplayer", true);
        if (player == null || world == null) {
            return result;
        }
        Object screen = field(mc, "currentScreen", "field_71462_r");
        result.addProperty("screen", screen == null ? "" : screen.getClass().getName());
        result.addProperty("dimension", dimensionId(world));
        result.add("player", playerJson(player));
        result.add("target", targetJson(mc, world));
        result.add("inventorySummary", inventorySummary(inventoryObject(player)));
        return result;
    }

    private JsonObject inventory() throws Exception {
        Object player = requirePlayer();
        Object inventory = inventoryObject(player);
        JsonObject result = ok();
        int selectedSlot = intField(inventory, 0, "currentItem", "field_70461_c");
        result.addProperty("selectedSlot", selectedSlot);
        JsonArray slots = new JsonArray();
        for (int index = 0; index < 41; index++) {
            slots.add(stackJson(index, stackAt(inventory, index), selectedSlot));
        }
        result.add("slots", slots);
        return result;
    }

    private JsonObject worldSnapshot(HttpExchange exchange) throws Exception {
        Object player = requirePlayer();
        Object world = requireWorld();
        int radius = clamp(queryInt(exchange, "radius", 2), 0, MAX_SNAPSHOT_RADIUS);
        boolean includeAir = queryBool(exchange, "includeAir", false);
        BlockPos center = queryBlockPos(exchange, playerBlockPos(player));

        JsonObject result = ok();
        result.addProperty("dimension", dimensionId(world));
        result.add("center", blockPosJson(center));
        result.addProperty("radius", radius);
        result.addProperty("includeAir", includeAir);

        JsonArray blocks = new JsonArray();
        for (int y = center.y - radius; y <= center.y + radius; y++) {
            for (int z = center.z - radius; z <= center.z + radius; z++) {
                for (int x = center.x - radius; x <= center.x + radius; x++) {
                    BlockPos pos = new BlockPos(x, y, z);
                    JsonObject block = blockJson(world, pos);
                    if (!includeAir && "minecraft:air".equals(block.get("id").getAsString())) {
                        continue;
                    }
                    blocks.add(block);
                }
            }
        }
        result.add("blocks", blocks);
        return result;
    }

    private JsonObject openWorld(final JsonObject body) throws Exception {
        return clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                Object mc = requireMinecraft();
                disablePauseOnLostFocus();
                Object player = field(mc, "player", "thePlayer", "field_71439_g");
                Object world = field(mc, "world", "theWorld", "field_71441_e");
                String requestedName = optionalString(body, "world", optionalString(body, "name", "Power Mine Test World"));
                String saveName = cleanWorldName(optionalString(body, "saveName", requestedName));
                String displayName = optionalString(body, "displayName", optionalString(body, "display_name", requestedName));
                boolean create = optionalBool(body, "create", true);
                if (player != null && world != null) {
                    JsonObject result = ok();
                    result.addProperty("loaded", true);
                    result.addProperty("alreadyLoaded", true);
                    result.addProperty("world", saveName);
                    return result;
                }

                Object settings = create ? worldSettings(body) : null;
                call(
                        mc,
                        new String[]{"launchIntegratedServer", "func_71371_a"},
                        new Class<?>[]{String.class, String.class, worldSettingsClass()},
                        saveName,
                        displayName,
                        settings
                );

                JsonObject result = ok();
                result.addProperty("loaded", false);
                result.addProperty("opening", true);
                result.addProperty("world", saveName);
                result.addProperty("displayName", displayName);
                result.addProperty("create", create);
                return result;
            }
        });
    }

    private JsonObject releaseInput() throws Exception {
        return clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                Object mc = requireMinecraft();
                boolean changed = disablePauseOnLostFocus();
                boolean mouseReleased = releaseMouse(mc);

                JsonObject result = ok();
                result.addProperty("pauseOnLostFocus", pauseOnLostFocus());
                result.addProperty("pauseOnLostFocusChanged", changed);
                result.addProperty("mouseReleased", mouseReleased);
                return result;
            }
        });
    }

    private boolean disablePauseOnLostFocus() {
        try {
            Object mc = requireMinecraft();
            Object settings = field(mc, "gameSettings", "field_71474_y", "u");
            boolean previous = booleanField(settings, true, "pauseOnLostFocus", "field_82882_x", "aO");
            setField(settings, false, "pauseOnLostFocus", "field_82882_x", "aO");
            return previous;
        } catch (Exception ignored) {
            return false;
        }
    }

    private boolean pauseOnLostFocus() {
        try {
            Object mc = requireMinecraft();
            Object settings = field(mc, "gameSettings", "field_71474_y", "u");
            return booleanField(settings, false, "pauseOnLostFocus", "field_82882_x", "aO");
        } catch (Exception ignored) {
            return false;
        }
    }

    private boolean releaseMouse(Object mc) {
        try {
            call(mc, new String[]{"setIngameNotInFocus", "func_71364_i", "l"}, new Class<?>[0]);
            return true;
        } catch (Exception ignored) {
        }
        try {
            Object mouseHelper = field(mc, "mouseHelper", "field_71417_B", "P");
            call(mouseHelper, new String[]{"ungrabMouseCursor", "func_74373_b", "b"}, new Class<?>[0]);
            setOptionalField(mc, false, "inGameHasFocus", "field_71415_G", "ap");
            return true;
        } catch (Exception ignored) {
        }
        try {
            setOptionalField(mc, false, "inGameHasFocus", "field_71415_G", "ap");
            return true;
        } catch (Exception ignored) {
            return false;
        }
    }

    private JsonObject selectHotbar(JsonObject body) throws Exception {
        Object inventory = inventoryObject(requirePlayer());
        int slot = requireInt(body, "slot");
        if (slot < 0 || slot > 8) {
            throw new IllegalArgumentException("slot must be between 0 and 8");
        }
        setField(inventory, slot, "currentItem", "field_70461_c");
        JsonObject result = ok();
        result.addProperty("selectedSlot", slot);
        return result;
    }

    private JsonObject giveItem(JsonObject body) throws Exception {
        Object player = requirePlayer();
        Object inventory = inventoryObject(player);
        String itemId = optionalString(body, "item", optionalString(body, "id", ""));
        if (itemId.length() == 0) {
            throw new IllegalArgumentException("missing item id: pass item or id");
        }
        Object item = findGameObject(true, itemId);
        if (item == null) {
            throw new IllegalArgumentException("unknown item id: " + itemId);
        }
        int slot = optionalInt(body, "slot", intField(inventory, 0, "currentItem", "field_70461_c"));
        if (slot < 0 || slot >= 41) {
            throw new IllegalArgumentException("slot must be between 0 and 40 for Forge 1.12.2");
        }
        int maxStack = intCall(item, 64, "getItemStackLimit", "func_77639_j");
        int count = clamp(optionalInt(body, "count", 1), 1, Math.max(1, maxStack));
        int damage = optionalInt(body, "damage", optionalInt(body, "metadata", 0));
        boolean select = optionalBool(body, "select", true);
        boolean replace = optionalBool(body, "replace", true);

        Object previous = stackAt(inventory, slot);
        if (!replace && previous != null) {
            throw new IllegalArgumentException("inventory slot is not empty: " + slot);
        }
        Object stack = newItemStack(item, count, damage);
        setStackAt(inventory, slot, stack);
        if (select && slot >= 0 && slot <= 8) {
            setField(inventory, slot, "currentItem", "field_70461_c");
        }
        markInventoryDirty(inventory, player);

        JsonObject result = ok();
        result.addProperty("item", itemId);
        result.addProperty("count", count);
        result.addProperty("slot", slot);
        result.addProperty("selected", select && slot >= 0 && slot <= 8);
        result.add("previous", stackJson(slot, previous, intField(inventory, 0, "currentItem", "field_70461_c")));
        result.add("stack", stackJson(slot, stackAt(inventory, slot), intField(inventory, 0, "currentItem", "field_70461_c")));
        return result;
    }

    private JsonObject placeBlock(JsonObject body) throws Exception {
        Object world = requireWorld();
        BlockPos pos = requireBlockPos(body);
        String blockId = optionalString(body, "block", "minecraft:stone");
        int metadata = optionalInt(body, "metadata", 0);
        Object block = findGameObject(false, blockId);
        if (block == null) {
            throw new IllegalArgumentException("unknown block id: " + blockId);
        }
        Object state = call(block, new String[]{"getStateFromMeta", "func_176203_a"}, new Class<?>[]{int.class}, metadata);
        Object blockPos = blockPosObject(pos);
        Boolean changed = (Boolean) call(
                world,
                new String[]{"setBlockState", "func_180501_a"},
                new Class<?>[]{blockPosClass(), iBlockStateClass(), int.class},
                blockPos,
                state,
                3
        );
        JsonObject result = ok();
        result.addProperty("changed", changed != null && changed);
        result.add("pos", blockPosJson(pos));
        JsonObject placed = blockJson(world, pos);
        result.addProperty("id", placed.get("id").getAsString());
        result.addProperty("metadata", placed.get("metadata").getAsInt());
        return result;
    }

    private JsonObject breakBlock(JsonObject body) throws Exception {
        Object world = requireWorld();
        BlockPos pos = requireBlockPos(body);
        boolean drop = optionalBool(body, "drop", false);
        Boolean changed;
        if (drop) {
            changed = (Boolean) call(
                    world,
                    new String[]{"destroyBlock", "func_175655_b"},
                    new Class<?>[]{blockPosClass(), boolean.class},
                    blockPosObject(pos),
                    true
            );
        } else {
            changed = (Boolean) call(
                    world,
                    new String[]{"setBlockToAir", "func_175698_g"},
                    new Class<?>[]{blockPosClass()},
                    blockPosObject(pos)
            );
        }
        JsonObject result = ok();
        result.addProperty("changed", changed != null && changed);
        result.add("pos", blockPosJson(pos));
        JsonObject current = blockJson(world, pos);
        result.addProperty("id", current.get("id").getAsString());
        result.addProperty("metadata", current.get("metadata").getAsInt());
        return result;
    }

    private JsonObject waitTicks(JsonObject body) throws Exception {
        int ticks = clamp(optionalInt(body, "ticks", 20), 0, 1200);
        int timeoutSeconds = clamp(optionalInt(body, "timeoutSeconds", Math.max(5, (ticks / 20) + 5)), 1, 300);
        long started = clientWorldTime();
        long target = started + ticks;
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(timeoutSeconds);
        long current = started;
        while (ticks > 0 && current < target && System.nanoTime() < deadline) {
            Thread.sleep(50);
            current = clientWorldTime();
        }

        boolean complete = current >= target;
        JsonObject result = ok();
        result.addProperty("ok", complete);
        result.addProperty("ticksRequested", ticks);
        result.addProperty("timeoutSeconds", timeoutSeconds);
        result.addProperty("startWorldTime", started);
        result.addProperty("currentWorldTime", current);
        result.addProperty("elapsedTicks", current - started);
        result.addProperty("complete", complete);
        if (!complete) {
            result.addProperty("error", "timed out waiting for Minecraft ticks");
        }
        return result;
    }

    private long clientWorldTime() throws Exception {
        JsonObject result = clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                Object world = requireWorld();
                JsonObject response = ok();
                response.addProperty("worldTime", longCall(world, 0L, "getTotalWorldTime", "func_82737_E"));
                return response;
            }
        });
        return result.get("worldTime").getAsLong();
    }

    private JsonObject useItem(final JsonObject body) throws Exception {
        return clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                Object mc = requireMinecraft();
                Object player = requirePlayer();
                Object world = requireWorld();
                Object inventory = inventoryObject(player);
                if (body.has("slot")) {
                    int slot = clamp(optionalInt(body, "slot", intField(inventory, 0, "currentItem", "field_70461_c")), 0, 8);
                    setField(inventory, slot, "currentItem", "field_70461_c");
                }
                String hand = normalizeHand(optionalString(body, "hand", "main"));

                int selectedSlot = intField(inventory, 0, "currentItem", "field_70461_c");
                int stackSlot = "offhand".equals(hand) ? 40 : selectedSlot;
                Object stack = stackAt(inventory, stackSlot);
                JsonObject before = stackJson(stackSlot, stack, selectedSlot);
                Boolean used = Boolean.FALSE;
                if (!isStackEmpty(stack)) {
                    Object controller = field(mc, "playerController", "field_71442_b");
                    Object value = call(
                            controller,
                            new String[]{"processRightClick", "func_187101_a"},
                            new Class<?>[]{entityPlayerClass(), worldClass(), enumHandClass()},
                            player,
                            world,
                            enumHand(hand)
                    );
                    used = actionResultUsed(value);
                    try {
                        call(player, new String[]{"swingArm", "func_184609_a"}, new Class<?>[]{enumHandClass()}, enumHand(hand));
                    } catch (Exception ignored) {
                    }
                    markInventoryDirty(inventory, player);
                }

                Object afterStack = stackAt(inventory, stackSlot);
                JsonObject result = ok();
                result.addProperty("hand", hand);
                result.addProperty("selectedSlot", selectedSlot);
                result.addProperty("used", used.booleanValue());
                result.addProperty("screen", currentScreenName(mc));
                result.add("before", before);
                result.add("after", stackJson(stackSlot, afterStack, selectedSlot));
                return result;
            }
        });
    }

    private JsonObject useBlock(final JsonObject body) throws Exception {
        return clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                Object mc = requireMinecraft();
                Object player = requirePlayer();
                Object world = requireWorld();
                Object inventory = inventoryObject(player);
                if (body.has("slot")) {
                    int slot = clamp(optionalInt(body, "slot", intField(inventory, 0, "currentItem", "field_70461_c")), 0, 8);
                    setField(inventory, slot, "currentItem", "field_70461_c");
                }
                String hand = normalizeHand(optionalString(body, "hand", "main"));

                BlockPos pos = requireBlockPos(body);
                int side = sideIndex(optionalString(body, "side", "up"));
                float hitX = optionalFloat(body, "hitX", 0.5f);
                float hitY = optionalFloat(body, "hitY", 0.5f);
                float hitZ = optionalFloat(body, "hitZ", 0.5f);
                int selectedSlot = intField(inventory, 0, "currentItem", "field_70461_c");
                int stackSlot = "offhand".equals(hand) ? 40 : selectedSlot;
                Object stack = stackAt(inventory, stackSlot);
                JsonObject beforeStack = stackJson(stackSlot, stack, selectedSlot);
                JsonObject beforeBlock = blockJson(world, pos);

                Object controller = field(mc, "playerController", "field_71442_b");
                Object hit = vec3(pos.x + hitX, pos.y + hitY, pos.z + hitZ);
                Object value = call(
                        controller,
                        new String[]{"processRightClickBlock", "func_187099_a"},
                        new Class<?>[]{entityPlayerSPClass(), worldClientClass(), blockPosClass(), enumFacingClass(), vec3Class(), enumHandClass()},
                        player,
                        world,
                        blockPosObject(pos),
                        enumFacing(side),
                        hit,
                        enumHand(hand)
                );
                Boolean used = actionResultUsed(value);
                try {
                    call(player, new String[]{"swingArm", "func_184609_a"}, new Class<?>[]{enumHandClass()}, enumHand(hand));
                } catch (Exception ignored) {
                }
                markInventoryDirty(inventory, player);

                Object afterStack = stackAt(inventory, stackSlot);
                JsonObject result = ok();
                result.add("pos", blockPosJson(pos));
                result.addProperty("hand", hand);
                result.addProperty("side", sideName(side));
                result.addProperty("sideIndex", side);
                result.addProperty("selectedSlot", selectedSlot);
                result.addProperty("used", used.booleanValue());
                result.addProperty("screen", currentScreenName(mc));
                result.add("beforeBlock", beforeBlock);
                result.add("afterBlock", blockJson(world, pos));
                result.add("beforeStack", beforeStack);
                result.add("afterStack", stackJson(stackSlot, afterStack, selectedSlot));
                return result;
            }
        });
    }

    private JsonObject recipeCheck(JsonObject body) throws Exception {
        Object player = requirePlayer();
        Object world = requireWorld();
        int width = clamp(optionalInt(body, "width", 3), 1, 3);
        int height = clamp(optionalInt(body, "height", 3), 1, 3);
        boolean requireInventory = optionalBool(body, "requireInventory", false);
        String expectedOutput = optionalString(body, "expectedOutput", "");
        String expectedRecipe = optionalString(body, "recipeId", "");
        JsonArray itemSpecs = body.has("items") && body.get("items").isJsonArray() ? body.getAsJsonArray("items") : new JsonArray();

        Object grid = newCraftingGrid(player, width, height);
        JsonArray gridJson = new JsonArray();
        for (int index = 0; index < width * height; index++) {
            Object stack = stackFromGridSpec(itemSpecs.size() > index ? itemSpecs.get(index) : null);
            call(
                    grid,
                    new String[]{"setInventorySlotContents", "func_70299_a", "a"},
                    new Class<?>[]{int.class, itemStackClass()},
                    index,
                    stack == null ? emptyStack() : stack
            );
            gridJson.add(stackJson(index, stack, -1));
        }

        Object recipe = findMatchingRecipe(null, grid, world);
        Object output = null;
        if (recipe != null) {
            output = call(recipe, new String[]{"getCraftingResult", "func_77572_b", "a"}, new Class<?>[]{craftingInventoryClass()}, grid);
        }
        if (output == null) {
            output = callStatic(
                    craftingManagerClass(),
                    new String[]{"findMatchingResult", "func_82787_a"},
                    new Class<?>[]{craftingInventoryClass(), worldClass()},
                    grid,
                    world
            );
        }

        boolean availableFromInventory = inventoryContains(inventoryObject(player), gridJson);
        JsonObject result = ok();
        result.addProperty("width", width);
        result.addProperty("height", height);
        result.add("grid", gridJson);
        result.addProperty("matched", output != null);
        result.addProperty("craftable", output != null && (!requireInventory || availableFromInventory));
        result.addProperty("requireInventory", requireInventory);
        result.addProperty("availableFromInventory", availableFromInventory);
        if (recipe != null) {
            result.addProperty("recipeId", recipeRegistryName(recipe));
            result.addProperty("recipeClass", recipe.getClass().getName());
            result.addProperty("recipeSize", intCall(recipe, 0, "getRecipeSize", "func_77570_a", "a"));
        }
        if (output != null) {
            result.add("output", stackJson(-1, output, -1));
            String outputId = stackItemId(output);
            if (expectedOutput.length() > 0) {
                result.addProperty("expectedOutput", expectedOutput);
                result.addProperty("expectedOutputMatches", expectedOutput.equals(outputId));
            }
        } else if (expectedOutput.length() > 0) {
            result.addProperty("expectedOutput", expectedOutput);
            result.addProperty("expectedOutputMatches", false);
        }
        if (expectedRecipe.length() > 0) {
            result.addProperty("expectedRecipe", expectedRecipe);
            result.addProperty("recipeIdMatches", recipe != null && (expectedRecipe.equals(recipeRegistryName(recipe)) || expectedRecipe.equals(recipe.getClass().getName())));
        }
        return result;
    }

    private JsonObject craftRecipe(final JsonObject body) throws Exception {
        return clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                Object player = requirePlayer();
                Object world = requireWorld();
                Object inventory = inventoryObject(player);
                int width = clamp(optionalInt(body, "width", 3), 1, 3);
                int height = clamp(optionalInt(body, "height", 3), 1, 3);
                int crafts = clamp(optionalInt(body, "crafts", 1), 1, 64);
                boolean requireInventory = optionalBool(body, "requireInventory", true);
                boolean consume = optionalBool(body, "consume", true);
                boolean insertOutput = optionalBool(body, "insertOutput", true);
                int outputSlot = optionalInt(body, "outputSlot", -1);
                boolean replaceOutput = optionalBool(body, "replaceOutput", false);
                String expectedOutput = optionalString(body, "expectedOutput", "");
                JsonArray itemSpecs = body.has("items") && body.get("items").isJsonArray() ? body.getAsJsonArray("items") : new JsonArray();

                Object grid = newCraftingGrid(player, width, height);
                JsonArray gridJson = new JsonArray();
                for (int index = 0; index < width * height; index++) {
                    Object stack = stackFromGridSpec(itemSpecs.size() > index ? itemSpecs.get(index) : null);
                    call(
                            grid,
                            new String[]{"setInventorySlotContents", "func_70299_a", "a"},
                            new Class<?>[]{int.class, itemStackClass()},
                            index,
                            stack == null ? emptyStack() : stack
                    );
                    gridJson.add(stackJson(index, stack, -1));
                }

                Object recipe = findMatchingRecipe(null, grid, world);
                Object output = null;
                if (recipe != null) {
                    output = call(recipe, new String[]{"getCraftingResult", "func_77572_b", "a"}, new Class<?>[]{craftingInventoryClass()}, grid);
                }
                if (output == null) {
                    output = callStatic(
                            craftingManagerClass(),
                            new String[]{"findMatchingResult", "func_82787_a"},
                            new Class<?>[]{craftingInventoryClass(), worldClass()},
                            grid,
                            world
                    );
                }

                JsonObject result = ok();
                result.addProperty("width", width);
                result.addProperty("height", height);
                result.addProperty("crafts", crafts);
                result.addProperty("requireInventory", requireInventory);
                result.addProperty("consume", consume);
                result.addProperty("insertOutput", insertOutput);
                result.add("grid", gridJson);
                result.addProperty("matched", output != null);
                if (expectedOutput.length() > 0) {
                    result.addProperty("expectedOutput", expectedOutput);
                }

                if (recipe != null) {
                    result.addProperty("recipeId", recipeRegistryName(recipe));
                    result.addProperty("recipeClass", recipe.getClass().getName());
                    result.addProperty("recipeSize", intCall(recipe, 0, "getRecipeSize", "func_77570_a", "a"));
                }
                if (output == null) {
                    if (expectedOutput.length() > 0) {
                        result.addProperty("expectedOutputMatches", false);
                    }
                    result.addProperty("ok", false);
                    result.addProperty("crafted", false);
                    result.addProperty("error", "no matching recipe");
                    return result;
                }

                result.add("output", stackJson(-1, output, -1));
                String outputId = stackItemId(output);
                if (expectedOutput.length() > 0) {
                    result.addProperty("expectedOutputMatches", expectedOutput.equals(outputId));
                }

                Map<String, Integer> required = requiredStacks(gridJson, crafts);
                result.add("requiredItems", stackRequirementJson(required));
                JsonArray missing = missingItems(inventory, required);
                result.add("missingItems", missing);
                boolean availableFromInventory = missing.size() == 0;
                result.addProperty("availableFromInventory", availableFromInventory);
                if (requireInventory && !availableFromInventory) {
                    result.addProperty("ok", false);
                    result.addProperty("crafted", false);
                    result.addProperty("error", "missing required ingredients");
                    return result;
                }

                int outputCount = stackCount(output) * crafts;
                int maxStack = intCall(output, 64, "getMaxStackSize", "func_77976_d", "d");
                if (outputCount > Math.max(1, maxStack)) {
                    result.addProperty("ok", false);
                    result.addProperty("crafted", false);
                    result.addProperty("error", "crafted output does not fit in one stack: " + outputCount);
                    return result;
                }
                setStackCount(output, outputCount);

                if (consume && !required.isEmpty()) {
                    JsonArray consumed = new JsonArray();
                    consumeInventory(inventory, required, consumed);
                    result.add("consumedItems", consumed);
                } else {
                    result.add("consumedItems", new JsonArray());
                }

                int insertedSlot = -1;
                if (insertOutput) {
                    insertedSlot = insertInventoryStack(inventory, output, outputSlot, replaceOutput);
                    if (insertedSlot < 0) {
                        result.addProperty("ok", false);
                        result.addProperty("crafted", false);
                        result.addProperty("error", insertedSlot == -2 ? "output slot is not empty" : "no empty inventory slot for crafted output");
                        return result;
                    }
                }

                markInventoryDirty(inventory, player);
                result.addProperty("crafted", true);
                result.addProperty("outputSlot", insertedSlot);
                result.add("craftedStack", stackJson(insertedSlot, output, intField(inventory, 0, "currentItem", "field_70461_c")));
                return result;
            }
        });
    }

    private JsonObject heldItemRender(final HttpExchange exchange) throws Exception {
        final String requestedHand = normalizeHand(queryString(exchange, "hand", "main"));
        sleepForRenderFrame(queryInt(exchange, "delayMs", 250));
        return clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                Object player = requirePlayer();
                Object inventory = inventoryObject(player);
                int selectedSlot = intField(inventory, 0, "currentItem", "field_70461_c");
                int stackSlot = "offhand".equals(requestedHand) ? 40 : selectedSlot;
                Object stack = stackAt(inventory, stackSlot);
                ScreenshotCapture capture = captureScreenshot("held-item");
                int[] bounds = heldItemProbeBounds(capture.image.getWidth(), capture.image.getHeight());

                JsonObject result = ok();
                result.addProperty("hand", requestedHand);
                result.addProperty("selectedSlot", selectedSlot);
                result.add("stack", stackJson(stackSlot, stack, selectedSlot));
                result.add("render", visualRenderJson(capture, bounds, "held_item_lower_right"));
                return result;
            }
        });
    }

    private JsonObject blockRender(final HttpExchange exchange) throws Exception {
        final Integer queryX = queryOptionalInt(exchange, "x");
        final Integer queryY = queryOptionalInt(exchange, "y");
        final Integer queryZ = queryOptionalInt(exchange, "z");
        final boolean lookAt = queryBool(exchange, "lookAt", true);
        final JsonObject target = clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                Object player = requirePlayer();
                Object world = requireWorld();
                BlockPos fallback = playerBlockPos(player);
                BlockPos pos = new BlockPos(
                        queryX == null ? fallback.x : queryX,
                        queryY == null ? fallback.y : queryY,
                        queryZ == null ? fallback.z : queryZ
                );
                if (lookAt) {
                    lookAtBlock(player, pos);
                }

                JsonObject result = ok();
                result.add("pos", blockPosJson(pos));
                JsonObject block = blockJson(world, pos);
                result.addProperty("id", block.get("id").getAsString());
                result.addProperty("metadata", block.get("metadata").getAsInt());
                result.addProperty("state", block.get("state").getAsString());
                result.addProperty("cameraLookAt", lookAt);
                return result;
            }
        });

        sleepForRenderFrame(queryInt(exchange, "delayMs", 350));
        return clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                ScreenshotCapture capture = captureScreenshot("block-render");
                int[] bounds = centerProbeBounds(capture.image.getWidth(), capture.image.getHeight());

                JsonObject result = ok();
                result.add("pos", target.get("pos"));
                result.addProperty("id", target.get("id").getAsString());
                result.addProperty("metadata", target.get("metadata").getAsInt());
                result.addProperty("state", target.get("state").getAsString());
                result.addProperty("cameraLookAt", target.get("cameraLookAt").getAsBoolean());
                result.add("render", visualRenderJson(capture, bounds, "center_crosshair_block"));
                return result;
            }
        });
    }

    private JsonObject screenshot() throws Exception {
        return clientTask(new ClientTask() {
            @Override
            public JsonObject run() throws Exception {
                JsonObject result = ok();
                addScreenshotFields(result, captureScreenshot("screenshot"));
                return result;
            }
        });
    }

    private ScreenshotCapture captureScreenshot(String prefix) throws Exception {
        Object mc = requireMinecraft();
        int width = intField(mc, 0, "displayWidth", "field_71443_c", "d");
        int height = intField(mc, 0, "displayHeight", "field_71440_d", "e");
        if (width <= 0 || height <= 0) {
            throw new IllegalStateException("Minecraft display size is not ready");
        }
        Object framebuffer = call(mc, new String[]{"getFramebuffer", "func_147110_a", "a"}, new Class<?>[0]);
        File runDirectory = (File) field(mc, "mcDataDir", "field_71412_D", "w");
        File baseDirectory = new File(runDirectory, "power-mine-agent");
        File screenshotsDirectory = new File(baseDirectory, "screenshots");
        if (!screenshotsDirectory.isDirectory() && !screenshotsDirectory.mkdirs()) {
            throw new IOException("could not create screenshot directory: " + screenshotsDirectory);
        }

        String fileName = prefix + "-" + System.currentTimeMillis() + ".png";
        callStatic(
                screenshotHelperClass(),
                new String[]{"saveScreenshot", "func_148259_a", "a"},
                new Class<?>[]{File.class, String.class, int.class, int.class, framebufferClass()},
                baseDirectory,
                fileName,
                width,
                height,
                framebuffer
        );
        File path = new File(screenshotsDirectory, fileName);
        if (!path.isFile()) {
            throw new IOException("screenshot was not written: " + path);
        }

        BufferedImage image = ImageIO.read(path);
        if (image == null) {
            throw new IOException("screenshot is not a readable PNG: " + path);
        }

        int sampledPixels = 0;
        int nonZeroSamples = 0;
        int stepX = Math.max(1, image.getWidth() / 32);
        int stepY = Math.max(1, image.getHeight() / 32);
        for (int y = 0; y < image.getHeight(); y += stepY) {
            for (int x = 0; x < image.getWidth(); x += stepX) {
                sampledPixels++;
                if ((image.getRGB(x, y) & 0x00ffffff) != 0) {
                    nonZeroSamples++;
                }
            }
        }
        return new ScreenshotCapture(path, image, sampledPixels, nonZeroSamples);
    }

    private void addScreenshotFields(JsonObject result, ScreenshotCapture capture) {
        result.addProperty("path", capture.path.getAbsolutePath());
        result.addProperty("width", capture.image.getWidth());
        result.addProperty("height", capture.image.getHeight());
        result.addProperty("sampledPixels", capture.sampledPixels);
        result.addProperty("nonZeroSamples", capture.nonZeroSamples);
        result.addProperty("blankLikely", capture.nonZeroSamples == 0);
    }

    private JsonObject screenshotJson(ScreenshotCapture capture) {
        JsonObject result = new JsonObject();
        addScreenshotFields(result, capture);
        return result;
    }

    private JsonObject visualRenderJson(ScreenshotCapture capture, int[] bounds, String regionName) {
        JsonObject crop = imageRegionStats(capture.image, bounds[0], bounds[1], bounds[2], bounds[3]);
        crop.addProperty("region", regionName);

        JsonObject check = new JsonObject();
        check.addProperty("type", "screenshot-visual-probe");
        check.addProperty("visibleLikely", crop.get("visibleLikely").getAsBoolean());
        check.addProperty("confidence", crop.get("confidence").getAsString());
        check.addProperty("reason", crop.get("reason").getAsString());

        JsonObject result = new JsonObject();
        result.addProperty("type", "screenshot-visual-probe");
        result.addProperty("bakedModel", false);
        result.addProperty("model", "");
        result.add("screenshot", screenshotJson(capture));
        result.add("crop", crop);
        result.add("visualCheck", check);
        return result;
    }

    private JsonObject imageRegionStats(BufferedImage image, int x, int y, int width, int height) {
        x = clamp(x, 0, Math.max(0, image.getWidth() - 1));
        y = clamp(y, 0, Math.max(0, image.getHeight() - 1));
        width = clamp(width, 1, image.getWidth() - x);
        height = clamp(height, 1, image.getHeight() - y);

        int step = Math.max(1, Math.min(width, height) / 160);
        int samples = 0;
        int nonZeroSamples = 0;
        int edgeSamples = 0;
        int edgeHits = 0;
        double sum = 0.0;
        double sumSquares = 0.0;
        Set<Integer> colors = new HashSet<>();

        for (int py = y; py < y + height; py += step) {
            for (int px = x; px < x + width; px += step) {
                int rgb = image.getRGB(px, py) & 0x00ffffff;
                int luma = luma(rgb);
                samples++;
                if (rgb != 0) {
                    nonZeroSamples++;
                }
                sum += luma;
                sumSquares += (double) luma * (double) luma;
                colors.add(rgb & 0x00f0f0f0);
                if (px - step >= x) {
                    edgeSamples++;
                    if (Math.abs(luma - luma(image.getRGB(px - step, py) & 0x00ffffff)) > 18) {
                        edgeHits++;
                    }
                }
                if (py - step >= y) {
                    edgeSamples++;
                    if (Math.abs(luma - luma(image.getRGB(px, py - step) & 0x00ffffff)) > 18) {
                        edgeHits++;
                    }
                }
            }
        }

        double mean = samples == 0 ? 0.0 : sum / samples;
        double variance = samples == 0 ? 0.0 : Math.max(0.0, (sumSquares / samples) - (mean * mean));
        double stdDev = Math.sqrt(variance);
        double nonZeroRatio = samples == 0 ? 0.0 : (double) nonZeroSamples / (double) samples;
        double edgeRatio = edgeSamples == 0 ? 0.0 : (double) edgeHits / (double) edgeSamples;
        boolean visibleLikely = nonZeroRatio > 0.05 && (stdDev > 5.0 || edgeRatio > 0.001 || colors.size() > 3);
        String confidence = visibleLikely && (stdDev > 20.0 || edgeRatio > 0.08 || colors.size() > 48) ? "medium" : "low";
        String reason = visibleLikely
                ? "crop has nonblank color and enough contrast/edges for a visual probe"
                : "crop is blank or too flat for a reliable visual probe";

        JsonObject bounds = new JsonObject();
        bounds.addProperty("x", x);
        bounds.addProperty("y", y);
        bounds.addProperty("width", width);
        bounds.addProperty("height", height);

        JsonObject result = new JsonObject();
        result.add("bounds", bounds);
        result.addProperty("sampleStep", step);
        result.addProperty("pixelSamples", samples);
        result.addProperty("nonZeroSamples", nonZeroSamples);
        result.addProperty("nonZeroRatio", round(nonZeroRatio));
        result.addProperty("meanLuma", round(mean));
        result.addProperty("lumaStdDev", round(stdDev));
        result.addProperty("edgeSamples", edgeSamples);
        result.addProperty("edgeHits", edgeHits);
        result.addProperty("edgeRatio", round(edgeRatio));
        result.addProperty("uniqueQuantizedColors", colors.size());
        result.addProperty("visibleLikely", visibleLikely);
        result.addProperty("confidence", confidence);
        result.addProperty("reason", reason);
        return result;
    }

    private int luma(int rgb) {
        int r = (rgb >> 16) & 0xff;
        int g = (rgb >> 8) & 0xff;
        int b = rgb & 0xff;
        return (r * 299 + g * 587 + b * 114) / 1000;
    }

    private double round(double value) {
        return Math.round(value * 10000.0) / 10000.0;
    }

    private int[] heldItemProbeBounds(int width, int height) {
        int x = (int) Math.round(width * 0.52);
        int y = (int) Math.round(height * 0.42);
        int w = Math.max(1, (int) Math.round(width * 0.46));
        int h = Math.max(1, (int) Math.round(height * 0.52));
        return new int[]{x, y, w, h};
    }

    private int[] centerProbeBounds(int width, int height) {
        int size = Math.max(32, Math.min(width, height) / 2);
        int x = Math.max(0, (width - size) / 2);
        int y = Math.max(0, (height - size) / 2);
        return new int[]{x, y, size, size};
    }

    private void lookAtBlock(Object player, BlockPos pos) throws Exception {
        double eyeX = doubleField(player, 0, "posX", "field_70165_t");
        double eyeY = doubleField(player, 0, "posY", "field_70163_u") + floatCall(player, 1.62f, "getEyeHeight", "func_70047_e");
        double eyeZ = doubleField(player, 0, "posZ", "field_70161_v");
        double dx = (pos.x + 0.5) - eyeX;
        double dy = (pos.y + 0.5) - eyeY;
        double dz = (pos.z + 0.5) - eyeZ;
        double horizontal = Math.sqrt(dx * dx + dz * dz);
        float yaw = (float) (Math.atan2(dz, dx) * 180.0 / Math.PI) - 90.0f;
        float pitch = (float) (-(Math.atan2(dy, horizontal) * 180.0 / Math.PI));
        setField(player, yaw, "rotationYaw", "field_70177_z");
        setField(player, pitch, "rotationPitch", "field_70125_A");
        setOptionalField(player, yaw, "prevRotationYaw", "field_70126_B");
        setOptionalField(player, pitch, "prevRotationPitch", "field_70127_C");
        setOptionalField(player, yaw, "rotationYawHead", "field_70759_as");
        setOptionalField(player, yaw, "renderYawOffset", "field_70761_aq");
    }

    private void sleepForRenderFrame(int millis) throws InterruptedException {
        millis = clamp(millis, 0, 2000);
        if (millis <= 0) {
            return;
        }
        Thread.sleep(millis);
    }

    private JsonObject playerJson(Object player) throws Exception {
        JsonObject result = new JsonObject();
        result.addProperty("name", stringCall(player, "", "getName", "getCommandSenderName", "func_70005_c_"));
        result.add("pos", vecJson(
                doubleField(player, 0, "posX", "field_70165_t"),
                doubleField(player, 0, "posY", "field_70163_u"),
                doubleField(player, 0, "posZ", "field_70161_v")
        ));
        result.add("blockPos", blockPosJson(playerBlockPos(player)));
        result.addProperty("yaw", floatField(player, 0, "rotationYaw", "field_70177_z"));
        result.addProperty("pitch", floatField(player, 0, "rotationPitch", "field_70125_A"));
        result.addProperty("health", floatCall(player, 0, "getHealth", "func_110143_aJ"));
        Object inventory = inventoryObject(player);
        result.addProperty("selectedSlot", intField(inventory, 0, "currentItem", "field_70461_c"));
        return result;
    }

    private JsonObject targetJson(Object mc, Object world) throws Exception {
        JsonObject result = new JsonObject();
        Object target = field(mc, "objectMouseOver", "field_71476_x");
        if (target == null) {
            result.addProperty("type", "none");
            return result;
        }
        Object type = field(target, "typeOfHit", "field_72313_a");
        result.addProperty("type", type == null ? "unknown" : type.toString().toLowerCase(Locale.ROOT));
        Object blockPos = null;
        try {
            blockPos = call(target, new String[]{"getBlockPos", "func_178782_a"}, new Class<?>[0]);
        } catch (Exception ignored) {
        }
        if (blockPos != null) {
            BlockPos pos = blockPosFromObject(blockPos);
            result.add("pos", blockPosJson(pos));
            JsonObject block = blockJson(world, pos);
            result.addProperty("id", block.get("id").getAsString());
            result.addProperty("metadata", block.get("metadata").getAsInt());
        }
        return result;
    }

    private String currentScreenName(Object mc) {
        try {
            Object screen = field(mc, "currentScreen", "field_71462_r");
            return screen == null ? "" : screen.getClass().getName();
        } catch (Exception ignored) {
            return "";
        }
    }

    private JsonObject inventorySummary(Object inventory) throws Exception {
        JsonObject result = new JsonObject();
        int occupied = 0;
        for (int index = 0; index < 41; index++) {
            if (!isStackEmpty(stackAt(inventory, index))) {
                occupied++;
            }
        }
        result.addProperty("slots", 41);
        result.addProperty("occupied", occupied);
        result.addProperty("selectedSlot", intField(inventory, 0, "currentItem", "field_70461_c"));
        return result;
    }

    private JsonObject stackJson(int index, Object stack, int selectedSlot) {
        JsonObject result = new JsonObject();
        result.addProperty("slot", index);
        result.addProperty("slotType", slotType(index));
        result.addProperty("selected", index == selectedSlot);
        boolean empty = isStackEmpty(stack);
        result.addProperty("empty", empty);
        if (!empty) {
            try {
                result.addProperty("id", stackItemId(stack));
                result.addProperty("name", stringCall(stack, "", "getDisplayName", "func_82833_r"));
                result.addProperty("count", stackCount(stack));
                result.addProperty("damage", stackDamage(stack));
                result.addProperty("maxDamage", intCall(stack, 0, "getMaxDamage", "func_77958_k", "l"));
            } catch (Exception exc) {
                result.addProperty("error", exc.getMessage());
            }
        }
        return result;
    }

    private JsonObject blockJson(Object world, BlockPos pos) throws Exception {
        Object state = call(world, new String[]{"getBlockState", "func_180495_p"}, new Class<?>[]{blockPosClass()}, blockPosObject(pos));
        Object block = call(state, new String[]{"getBlock", "func_177230_c"}, new Class<?>[0]);
        Integer metadata = (Integer) call(block, new String[]{"getMetaFromState", "func_176201_c"}, new Class<?>[]{iBlockStateClass()}, state);
        JsonObject result = blockPosJson(pos);
        result.addProperty("id", registryName(block, false));
        result.addProperty("metadata", metadata == null ? 0 : metadata);
        result.addProperty("state", registryName(block, false) + ":" + (metadata == null ? 0 : metadata));
        return result;
    }

    private Object minecraft() {
        try {
            Class<?> clazz = classByName("net.minecraft.client.Minecraft");
            return callStatic(clazz, new String[]{"getMinecraft", "func_71410_x"}, new Class<?>[0]);
        } catch (Exception ignored) {
            return null;
        }
    }

    private Object requireMinecraft() {
        Object mc = minecraft();
        if (mc == null) {
            throw new IllegalStateException("Minecraft client is not ready");
        }
        return mc;
    }

    private Object requirePlayer() throws Exception {
        Object player = field(requireMinecraft(), "player", "thePlayer", "field_71439_g");
        if (player == null) {
            throw new IllegalStateException("Minecraft client is not in a loaded world");
        }
        return player;
    }

    private Object requireWorld() throws Exception {
        Object world = field(requireMinecraft(), "world", "theWorld", "field_71441_e");
        if (world == null) {
            throw new IllegalStateException("Minecraft client is not in a loaded world");
        }
        return world;
    }

    private Object inventoryObject(Object player) throws Exception {
        return field(player, "inventory", "field_71071_by");
    }

    private Object stackAt(Object inventory, int slot) throws Exception {
        Object container = inventorySlotContainer(inventory, slot);
        Object stack = listOrArrayGet(container, inventorySlotOffset(slot));
        return isStackEmpty(stack) ? null : stack;
    }

    private void setStackAt(Object inventory, int slot, Object stack) throws Exception {
        Object container = inventorySlotContainer(inventory, slot);
        listOrArraySet(container, inventorySlotOffset(slot), stack == null ? emptyStack() : stack);
    }

    private Object inventorySlotContainer(Object inventory, int slot) throws Exception {
        if (slot < 36) {
            return field(inventory, "mainInventory", "field_70462_a");
        }
        if (slot < 40) {
            return field(inventory, "armorInventory", "field_70460_b");
        }
        return field(inventory, "offHandInventory", "field_184439_c");
    }

    private int inventorySlotOffset(int slot) {
        if (slot < 36) {
            return slot;
        }
        if (slot < 40) {
            return slot - 36;
        }
        return 0;
    }

    private Object listOrArrayGet(Object container, int offset) {
        if (container instanceof List) {
            return ((List<?>) container).get(offset);
        }
        return Array.get(container, offset);
    }

    @SuppressWarnings({"rawtypes", "unchecked"})
    private void listOrArraySet(Object container, int offset, Object value) {
        if (container instanceof List) {
            ((List) container).set(offset, value);
            return;
        }
        Array.set(container, offset, value);
    }

    private Object emptyStack() throws Exception {
        return staticField(itemStackClass(), "EMPTY", "field_190927_a");
    }

    private boolean isStackEmpty(Object stack) {
        if (stack == null) {
            return true;
        }
        try {
            Object empty = emptyStack();
            if (stack == empty || stack.equals(empty)) {
                return true;
            }
        } catch (Exception ignored) {
        }
        return booleanCall(stack, false, "isEmpty", "func_190926_b");
    }

    private void markInventoryDirty(Object inventory, Object player) {
        try {
            call(inventory, new String[]{"markDirty", "func_70296_d"}, new Class<?>[0]);
        } catch (Exception ignored) {
        }
        try {
            Object container = field(player, "inventoryContainer", "field_71069_bz");
            call(container, new String[]{"detectAndSendChanges", "func_75142_b"}, new Class<?>[0]);
        } catch (Exception ignored) {
        }
    }

    private Object newItemStack(Object item, int count, int damage) throws Exception {
        Constructor<?> constructor = itemStackClass().getConstructor(itemClass(), int.class, int.class);
        return constructor.newInstance(item, count, damage);
    }

    private Object newBlockStack(Object block, int count, int damage) throws Exception {
        Constructor<?> constructor = itemStackClass().getConstructor(blockClass(), int.class, int.class);
        return constructor.newInstance(block, count, damage);
    }

    private Object newCraftingGrid(Object player, int width, int height) throws Exception {
        Object container = field(player, "inventoryContainer", "field_71069_bz", "bn", "bo");
        Constructor<?> constructor = craftingInventoryClass().getConstructor(containerClass(), int.class, int.class);
        return constructor.newInstance(container, width, height);
    }

    private Object vec3(double x, double y, double z) throws Exception {
        Constructor<?> constructor = vec3Class().getConstructor(double.class, double.class, double.class);
        return constructor.newInstance(x, y, z);
    }

    private Object blockPosObject(BlockPos pos) throws Exception {
        Constructor<?> constructor = blockPosClass().getConstructor(int.class, int.class, int.class);
        return constructor.newInstance(pos.x, pos.y, pos.z);
    }

    private BlockPos blockPosFromObject(Object pos) throws Exception {
        return new BlockPos(
                intCall(pos, 0, "getX", "func_177958_n"),
                intCall(pos, 0, "getY", "func_177956_o"),
                intCall(pos, 0, "getZ", "func_177952_p")
        );
    }

    private Object stackFromGridSpec(JsonElement spec) throws Exception {
        if (spec == null || spec.isJsonNull()) {
            return null;
        }
        String id = "";
        int count = 1;
        int damage = 0;
        if (spec.isJsonPrimitive()) {
            id = spec.getAsString().trim();
        } else if (spec.isJsonObject()) {
            JsonObject object = spec.getAsJsonObject();
            id = optionalString(object, "item", optionalString(object, "id", optionalString(object, "block", ""))).trim();
            count = clamp(optionalInt(object, "count", 1), 1, 64);
            damage = optionalInt(object, "damage", optionalInt(object, "metadata", 0));
        }
        if (id.length() == 0 || "minecraft:air".equals(id) || "air".equals(id)) {
            return null;
        }

        Object item = findGameObject(true, id);
        if (item != null) {
            return newItemStack(item, count, damage);
        }
        Object block = findGameObject(false, id);
        if (block != null) {
            return newBlockStack(block, count, damage);
        }
        throw new IllegalArgumentException("unknown recipe item or block id: " + id);
    }

    private Object findMatchingRecipe(Object manager, Object grid, Object world) throws Exception {
        try {
            Object recipe = callStatic(
                    craftingManagerClass(),
                    new String[]{"findMatchingRecipe", "func_192413_b"},
                    new Class<?>[]{craftingInventoryClass(), worldClass()},
                    grid,
                    world
            );
            if (recipe != null) {
                return recipe;
            }
        } catch (Exception ignored) {
        }
        Object recipes = forgeRecipeRegistryValues();
        if (!(recipes instanceof List)) {
            return null;
        }
        for (Object recipe : (List<?>) recipes) {
            Object matches = call(
                    recipe,
                    new String[]{"matches", "func_77569_a", "a"},
                    new Class<?>[]{craftingInventoryClass(), worldClass()},
                    grid,
                    world
            );
            if (matches instanceof Boolean && (Boolean) matches) {
                return recipe;
            }
        }
        return null;
    }

    private Object forgeRecipeRegistryValues() {
        try {
            Class<?> registries = Class.forName("net.minecraftforge.fml.common.registry.ForgeRegistries");
            Object registry = staticField(registries, "RECIPES");
            Object values = call(registry, new String[]{"getValues"}, new Class<?>[0]);
            if (values instanceof List) {
                return values;
            }
            if (values instanceof Iterable) {
                java.util.ArrayList<Object> list = new java.util.ArrayList<>();
                for (Object value : (Iterable<?>) values) {
                    list.add(value);
                }
                return list;
            }
        } catch (Exception ignored) {
        }
        return null;
    }

    private boolean inventoryContains(Object inventory, JsonArray grid) throws Exception {
        Map<String, Integer> needed = new HashMap<>();
        for (int index = 0; index < grid.size(); index++) {
            JsonElement element = grid.get(index);
            if (!element.isJsonObject()) {
                continue;
            }
            JsonObject stack = element.getAsJsonObject();
            if (stack.has("empty") && stack.get("empty").getAsBoolean()) {
                continue;
            }
            String id = optionalString(stack, "id", "");
            if (id.length() == 0 || "minecraft:air".equals(id)) {
                continue;
            }
            int count = optionalInt(stack, "count", 1);
            int damage = optionalInt(stack, "damage", 0);
            String key = id + ":" + damage;
            needed.put(key, needed.containsKey(key) ? needed.get(key) + count : count);
        }
        if (needed.isEmpty()) {
            return true;
        }

        Map<String, Integer> available = new HashMap<>();
        for (int slot = 0; slot < 41; slot++) {
            Object stack = stackAt(inventory, slot);
            if (isStackEmpty(stack)) {
                continue;
            }
            String key = stackItemId(stack) + ":" + stackDamage(stack);
            int count = stackCount(stack);
            available.put(key, available.containsKey(key) ? available.get(key) + count : count);
        }

        for (Map.Entry<String, Integer> entry : needed.entrySet()) {
            Integer count = available.get(entry.getKey());
            if (count == null || count < entry.getValue()) {
                return false;
            }
        }
        return true;
    }

    private Map<String, Integer> requiredStacks(JsonArray grid, int crafts) {
        Map<String, Integer> required = new HashMap<>();
        for (int index = 0; index < grid.size(); index++) {
            JsonElement element = grid.get(index);
            if (!element.isJsonObject()) {
                continue;
            }
            JsonObject stack = element.getAsJsonObject();
            if (stack.has("empty") && stack.get("empty").getAsBoolean()) {
                continue;
            }
            String id = optionalString(stack, "id", "");
            if (id.length() == 0 || "minecraft:air".equals(id)) {
                continue;
            }
            int count = optionalInt(stack, "count", 1);
            int damage = optionalInt(stack, "damage", 0);
            String key = forgeStackKey(id, damage);
            required.put(key, required.containsKey(key) ? required.get(key) + (count * crafts) : (count * crafts));
        }
        return required;
    }

    private Map<String, Integer> inventoryCounts(Object inventory) throws Exception {
        Map<String, Integer> available = new HashMap<>();
        for (int slot = 0; slot < 36; slot++) {
            Object stack = stackAt(inventory, slot);
            if (isStackEmpty(stack)) {
                continue;
            }
            String key = forgeStackKey(stackItemId(stack), stackDamage(stack));
            int count = stackCount(stack);
            available.put(key, available.containsKey(key) ? available.get(key) + count : count);
        }
        return available;
    }

    private JsonArray stackRequirementJson(Map<String, Integer> required) {
        JsonArray items = new JsonArray();
        for (Map.Entry<String, Integer> entry : required.entrySet()) {
            JsonObject item = forgeStackKeyJson(entry.getKey());
            item.addProperty("count", entry.getValue());
            items.add(item);
        }
        return items;
    }

    private JsonArray missingItems(Object inventory, Map<String, Integer> required) throws Exception {
        Map<String, Integer> available = inventoryCounts(inventory);
        JsonArray missing = new JsonArray();
        for (Map.Entry<String, Integer> entry : required.entrySet()) {
            int availableCount = available.containsKey(entry.getKey()) ? available.get(entry.getKey()) : 0;
            if (availableCount >= entry.getValue()) {
                continue;
            }
            JsonObject item = forgeStackKeyJson(entry.getKey());
            item.addProperty("required", entry.getValue());
            item.addProperty("available", availableCount);
            item.addProperty("missing", entry.getValue() - availableCount);
            missing.add(item);
        }
        return missing;
    }

    private void consumeInventory(Object inventory, Map<String, Integer> required, JsonArray consumed) throws Exception {
        Map<String, Integer> remaining = new HashMap<>(required);
        for (int slot = 0; slot < 36; slot++) {
            Object stack = stackAt(inventory, slot);
            if (isStackEmpty(stack)) {
                continue;
            }
            String key = forgeStackKey(stackItemId(stack), stackDamage(stack));
            int needed = remaining.containsKey(key) ? remaining.get(key) : 0;
            if (needed <= 0) {
                continue;
            }
            int current = stackCount(stack);
            int taken = Math.min(needed, current);
            JsonObject entry = stackJson(slot, stack, -1);
            entry.addProperty("consumed", taken);
            consumed.add(entry);
            int leftInStack = current - taken;
            if (leftInStack <= 0) {
                setStackAt(inventory, slot, null);
            } else {
                setStackCount(stack, leftInStack);
            }
            int leftNeeded = needed - taken;
            if (leftNeeded <= 0) {
                remaining.remove(key);
            } else {
                remaining.put(key, leftNeeded);
            }
        }
    }

    private int insertInventoryStack(Object inventory, Object output, int outputSlot, boolean replace) throws Exception {
        if (outputSlot >= 0) {
            if (outputSlot >= 36) {
                throw new IllegalArgumentException("output slot must be between 0 and 35 for crafted output");
            }
            Object previous = stackAt(inventory, outputSlot);
            if (!replace && previous != null) {
                return -2;
            }
            setStackAt(inventory, outputSlot, output);
            return outputSlot;
        }
        for (int slot = 0; slot < 36; slot++) {
            if (stackAt(inventory, slot) == null) {
                setStackAt(inventory, slot, output);
                return slot;
            }
        }
        return -1;
    }

    private void setStackCount(Object stack, int count) throws Exception {
        try {
            call(stack, new String[]{"setCount", "func_190920_e"}, new Class<?>[]{int.class}, count);
        } catch (Exception exc) {
            setField(stack, count, "stackSize", "field_77994_a", "b");
        }
    }

    private String forgeStackKey(String id, int damage) {
        return id + "|" + damage;
    }

    private JsonObject forgeStackKeyJson(String key) {
        String[] pieces = key.split("\\|", 2);
        JsonObject item = new JsonObject();
        item.addProperty("key", key);
        item.addProperty("id", pieces.length > 0 ? pieces[0] : key);
        item.addProperty("damage", pieces.length > 1 ? Integer.parseInt(pieces[1]) : 0);
        return item;
    }

    private String stackItemId(Object stack) throws Exception {
        Object item = call(stack, new String[]{"getItem", "func_77973_b", "b"}, new Class<?>[0]);
        return registryName(item, true);
    }

    private int stackDamage(Object stack) {
        try {
            Object value = call(stack, new String[]{"getItemDamage", "func_77960_j", "k"}, new Class<?>[0]);
            return value instanceof Number ? ((Number) value).intValue() : 0;
        } catch (Exception exc) {
            return intField(stack, 0, "f");
        }
    }

    private int stackCount(Object stack) {
        try {
            Object value = call(stack, new String[]{"getCount", "func_190916_E"}, new Class<?>[0]);
            return value instanceof Number ? ((Number) value).intValue() : 0;
        } catch (Exception exc) {
            return intField(stack, 1, "stackSize", "field_77994_a", "b");
        }
    }

    private Object worldSettings(JsonObject body) throws Exception {
        long seed = body.has("seed") ? body.get("seed").getAsLong() : System.currentTimeMillis();
        boolean structures = optionalBool(body, "structures", true);
        boolean hardcore = optionalBool(body, "hardcore", false);
        String gamemode = optionalString(body, "gamemode", optionalString(body, "gameMode", "creative"));
        Object gameType = enumConstant(gameTypeClass(), gamemode);
        Object worldType = staticField(worldTypeClass(), "DEFAULT", "field_77137_b");
        Constructor<?> constructor = worldSettingsClass().getConstructor(long.class, gameTypeClass(), boolean.class, boolean.class, worldTypeClass());
        return constructor.newInstance(seed, gameType, structures, hardcore, worldType);
    }

    private Object findGameObject(boolean item, String id) throws Exception {
        Class<?> registries = Class.forName("net.minecraftforge.fml.common.registry.ForgeRegistries");
        Object registry = staticField(registries, item ? "ITEMS" : "BLOCKS");
        Object resourceLocation = resourceLocation(id);
        Object value = call(registry, new String[]{"getValue"}, new Class<?>[]{resourceLocationClass()}, resourceLocation);
        if (isAirValue(value, item)) {
            return null;
        }
        return value;
    }

    private String registryName(Object object, boolean item) {
        if (object == null) {
            return item ? "minecraft:air" : "minecraft:air";
        }
        try {
            Object name = call(object, new String[]{"getRegistryName"}, new Class<?>[0]);
            return name == null ? object.toString() : name.toString();
        } catch (Exception exc) {
            return object.toString();
        }
    }

    private String recipeRegistryName(Object recipe) {
        if (recipe == null) {
            return "";
        }
        try {
            Object name = call(recipe, new String[]{"getRegistryName"}, new Class<?>[0]);
            return name == null ? "" : name.toString();
        } catch (Exception ignored) {
            return "";
        }
    }

    private boolean isAirValue(Object value, boolean item) {
        if (value == null) {
            return true;
        }
        String name = registryName(value, item);
        return "minecraft:air".equals(name) || "air".equals(name);
    }

    private Object resourceLocation(String id) throws Exception {
        Constructor<?> constructor = resourceLocationClass().getConstructor(String.class);
        return constructor.newInstance(id);
    }

    private String dimensionId(Object world) {
        try {
            Object provider = field(world, "provider", "field_73011_w");
            Object dimension = call(provider, new String[]{"getDimension"}, new Class<?>[0]);
            if (dimension instanceof Number) {
                return Integer.toString(((Number) dimension).intValue());
            }
            return Integer.toString(intField(provider, 0, "dimensionId", "field_76574_g"));
        } catch (Exception exc) {
            return "";
        }
    }

    private BlockPos playerBlockPos(Object player) throws Exception {
        return new BlockPos(
                floor(doubleField(player, 0, "posX", "field_70165_t")),
                floor(doubleField(player, 0, "posY", "field_70163_u")),
                floor(doubleField(player, 0, "posZ", "field_70161_v"))
        );
    }

    private int floor(double value) {
        int integer = (int) value;
        return value < integer ? integer - 1 : integer;
    }

    private String slotType(int index) {
        if (index >= 0 && index <= 8) {
            return "hotbar";
        }
        if (index >= 9 && index <= 35) {
            return "main";
        }
        if (index >= 36 && index <= 39) {
            return "armor";
        }
        if (index == 40) {
            return "offhand";
        }
        return "unknown";
    }

    private int sideIndex(String value) {
        String normalized = value == null ? "" : value.trim().toLowerCase(Locale.ROOT);
        if ("0".equals(normalized) || "down".equals(normalized)) {
            return 0;
        }
        if ("2".equals(normalized) || "north".equals(normalized)) {
            return 2;
        }
        if ("3".equals(normalized) || "south".equals(normalized)) {
            return 3;
        }
        if ("4".equals(normalized) || "west".equals(normalized)) {
            return 4;
        }
        if ("5".equals(normalized) || "east".equals(normalized)) {
            return 5;
        }
        return 1;
    }

    private String sideName(int side) {
        switch (side) {
            case 0:
                return "down";
            case 2:
                return "north";
            case 3:
                return "south";
            case 4:
                return "west";
            case 5:
                return "east";
            case 1:
            default:
                return "up";
        }
    }

    private Object enumFacing(int side) throws ClassNotFoundException {
        String name = sideName(side).toUpperCase(Locale.ROOT);
        Object[] constants = enumFacingClass().getEnumConstants();
        for (Object constant : constants) {
            if (constant.toString().equalsIgnoreCase(name)) {
                return constant;
            }
        }
        return constants.length == 0 ? null : constants[1];
    }

    private String normalizeHand(String hand) {
        String normalized = hand == null ? "" : hand.trim().toLowerCase(Locale.ROOT);
        if ("off".equals(normalized) || "offhand".equals(normalized) || "off_hand".equals(normalized)) {
            return "offhand";
        }
        return "main";
    }

    private Object enumHand(String hand) throws ClassNotFoundException {
        String normalized = normalizeHand(hand);
        String name = "offhand".equals(normalized) ? "OFF_HAND" : "MAIN_HAND";
        Object[] constants = enumHandClass().getEnumConstants();
        for (Object constant : constants) {
            if (constant.toString().equalsIgnoreCase(name)) {
                return constant;
            }
        }
        return constants.length == 0 ? null : constants[0];
    }

    private Boolean actionResultUsed(Object value) {
        if (value instanceof Boolean) {
            return (Boolean) value;
        }
        if (value == null) {
            return Boolean.FALSE;
        }
        String text = value.toString();
        return Boolean.valueOf(!"PASS".equalsIgnoreCase(text));
    }

    private Class<?> itemClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.item.Item", "adb");
    }

    private Class<?> blockClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.block.Block", "aji");
    }

    private Class<?> itemStackClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.item.ItemStack", "add");
    }

    private Class<?> entityPlayerClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.entity.player.EntityPlayer", "yz");
    }

    private Class<?> entityPlayerSPClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.client.entity.EntityPlayerSP");
    }

    private Class<?> worldClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.world.World", "ahb");
    }

    private Class<?> worldClientClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.client.multiplayer.WorldClient");
    }

    private Class<?> vec3Class() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.util.math.Vec3d", "net.minecraft.util.Vec3", "azw");
    }

    private Class<?> blockPosClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.util.math.BlockPos");
    }

    private Class<?> iBlockStateClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.block.state.IBlockState");
    }

    private Class<?> resourceLocationClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.util.ResourceLocation");
    }

    private Class<?> enumFacingClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.util.EnumFacing");
    }

    private Class<?> enumHandClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.util.EnumHand");
    }

    private Class<?> containerClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.inventory.Container", "zs");
    }

    private Class<?> craftingInventoryClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.inventory.InventoryCrafting", "aae");
    }

    private Class<?> craftingManagerClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.item.crafting.CraftingManager", "afe");
    }

    private Class<?> screenshotHelperClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.util.ScreenShotHelper", "net.minecraft.util.ScreenshotHelper", "bbp");
    }

    private Class<?> framebufferClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.client.shader.Framebuffer", "bmg");
    }

    private Class<?> worldSettingsClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.world.WorldSettings", "ahj");
    }

    private Class<?> gameTypeClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.world.GameType", "net.minecraft.world.WorldSettings$GameType", "ahj$a");
    }

    private Class<?> worldTypeClass() throws ClassNotFoundException {
        return classByNameAny("net.minecraft.world.WorldType", "ahm");
    }

    private Class<?> classByName(String name) throws ClassNotFoundException {
        return Class.forName(name);
    }

    private Class<?> classByNameAny(String... names) throws ClassNotFoundException {
        ClassNotFoundException last = null;
        for (String name : names) {
            try {
                return classByName(name);
            } catch (ClassNotFoundException exc) {
                last = exc;
            }
        }
        throw last == null ? new ClassNotFoundException(joinNames(names)) : last;
    }

    private Object enumConstant(Class<?> enumClass, String value) {
        String normalized = value == null ? "" : value.trim().toUpperCase(Locale.ROOT);
        if (normalized.equals("SURVIVAL") || normalized.equals("0")) {
            normalized = "SURVIVAL";
        } else if (normalized.equals("ADVENTURE") || normalized.equals("2")) {
            normalized = "ADVENTURE";
        } else {
            normalized = "CREATIVE";
        }
        Object[] constants = enumClass.getEnumConstants();
        for (Object constant : constants) {
            if (constant.toString().equalsIgnoreCase(normalized)) {
                return constant;
            }
        }
        return constants.length == 0 ? null : constants[0];
    }

    private Object staticField(Class<?> clazz, String... names) throws Exception {
        Field field = findField(clazz, names);
        return field.get(null);
    }

    private Object field(Object target, String... names) throws Exception {
        Field field = findField(target.getClass(), names);
        return field.get(target);
    }

    private void setField(Object target, Object value, String... names) throws Exception {
        Field field = findField(target.getClass(), names);
        field.set(target, value);
    }

    private void setOptionalField(Object target, Object value, String... names) {
        try {
            setField(target, value, names);
        } catch (Exception ignored) {
        }
    }

    private Field findField(Class<?> clazz, String... names) throws NoSuchFieldException {
        Class<?> current = clazz;
        while (current != null) {
            for (String name : names) {
                try {
                    Field field = current.getDeclaredField(name);
                    field.setAccessible(true);
                    return field;
                } catch (NoSuchFieldException ignored) {
                }
            }
            current = current.getSuperclass();
        }
        throw new NoSuchFieldException(joinNames(names));
    }

    private Object call(Object target, String[] names, Class<?>[] types, Object... args) throws Exception {
        Method method = findMethod(target.getClass(), names, types);
        return method.invoke(target, args);
    }

    private Object callStatic(Class<?> clazz, String[] names, Class<?>[] types, Object... args) throws Exception {
        Method method = findMethod(clazz, names, types);
        return method.invoke(null, args);
    }

    private JsonObject clientTask(final ClientTask task) throws Exception {
        Object mc = requireMinecraft();
        if (booleanCall(mc, false, "isCallingFromMinecraftThread", "func_152345_ab")) {
            return task.run();
        }
        final CompletableFuture<JsonObject> future = new CompletableFuture<>();
        Runnable runnable = new Runnable() {
            @Override
            public void run() {
                try {
                    future.complete(task.run());
                } catch (Throwable throwable) {
                    future.completeExceptionally(throwable);
                }
            }
        };
        try {
            Object scheduled = call(mc, new String[]{"addScheduledTask", "func_152344_a"}, new Class<?>[]{Runnable.class}, runnable);
            if (scheduled instanceof Future) {
                ((Future<?>) scheduled).get(30, TimeUnit.SECONDS);
            }
        } catch (NoSuchMethodException exc) {
            return task.run();
        }
        return future.get(30, TimeUnit.SECONDS);
    }

    private Method findMethod(Class<?> clazz, String[] names, Class<?>[] types) throws NoSuchMethodException {
        Class<?> current = clazz;
        while (current != null) {
            for (String name : names) {
                try {
                    Method method = current.getDeclaredMethod(name, types);
                    method.setAccessible(true);
                    return method;
                } catch (NoSuchMethodException ignored) {
                }
            }
            current = current.getSuperclass();
        }
        throw new NoSuchMethodException(joinNames(names));
    }

    private int intField(Object target, int fallback, String... names) {
        try {
            Object value = field(target, names);
            return value instanceof Number ? ((Number) value).intValue() : fallback;
        } catch (Exception exc) {
            return fallback;
        }
    }

    private double doubleField(Object target, double fallback, String... names) {
        try {
            Object value = field(target, names);
            return value instanceof Number ? ((Number) value).doubleValue() : fallback;
        } catch (Exception exc) {
            return fallback;
        }
    }

    private float floatField(Object target, float fallback, String... names) {
        try {
            Object value = field(target, names);
            return value instanceof Number ? ((Number) value).floatValue() : fallback;
        } catch (Exception exc) {
            return fallback;
        }
    }

    private int intCall(Object target, int fallback, String... names) {
        try {
            Object value = call(target, names, new Class<?>[0]);
            return value instanceof Number ? ((Number) value).intValue() : fallback;
        } catch (Exception exc) {
            return fallback;
        }
    }

    private long longCall(Object target, long fallback, String... names) {
        try {
            Object value = call(target, names, new Class<?>[0]);
            return value instanceof Number ? ((Number) value).longValue() : fallback;
        } catch (Exception exc) {
            return fallback;
        }
    }

    private boolean booleanCall(Object target, boolean fallback, String... names) {
        try {
            Object value = call(target, names, new Class<?>[0]);
            return value instanceof Boolean ? (Boolean) value : fallback;
        } catch (Exception exc) {
            return fallback;
        }
    }

    private boolean booleanField(Object target, boolean fallback, String... names) {
        try {
            Object value = field(target, names);
            return value instanceof Boolean ? (Boolean) value : fallback;
        } catch (Exception exc) {
            return fallback;
        }
    }

    private float floatCall(Object target, float fallback, String... names) {
        try {
            Object value = call(target, names, new Class<?>[0]);
            return value instanceof Number ? ((Number) value).floatValue() : fallback;
        } catch (Exception exc) {
            return fallback;
        }
    }

    private String stringCall(Object target, String fallback, String... names) {
        try {
            Object value = call(target, names, new Class<?>[0]);
            return value == null ? fallback : value.toString();
        } catch (Exception exc) {
            return fallback;
        }
    }

    private String[] splitIdentifier(String id) {
        String trimmed = id == null ? "" : id.trim();
        if (trimmed.length() == 0) {
            return new String[]{"minecraft", "air"};
        }
        int colon = trimmed.indexOf(':');
        if (colon < 0) {
            return new String[]{"minecraft", trimmed};
        }
        return new String[]{trimmed.substring(0, colon), trimmed.substring(colon + 1)};
    }

    private String cleanWorldName(String value) {
        String name = value == null ? "" : value.trim();
        if (name.length() == 0) {
            name = "Power Mine Test World";
        }
        return name.replaceAll("[\\\\/:*?\"<>|]", "_");
    }

    private JsonObject parseBody(HttpExchange exchange) throws IOException {
        try (InputStream stream = exchange.getRequestBody()) {
            ByteArrayOutputStream output = new ByteArrayOutputStream();
            byte[] buffer = new byte[4096];
            int read;
            while ((read = stream.read(buffer)) != -1) {
                output.write(buffer, 0, read);
            }
            byte[] raw = output.toByteArray();
            if (raw.length == 0) {
                return new JsonObject();
            }
            JsonElement element = new JsonParser().parse(new String(raw, StandardCharsets.UTF_8));
            if (!element.isJsonObject()) {
                throw new IllegalArgumentException("request body must be a JSON object");
            }
            return element.getAsJsonObject();
        }
    }

    private boolean authorized(HttpExchange exchange, String token) {
        if (token == null || token.trim().length() == 0) {
            return true;
        }
        String header = exchange.getRequestHeaders().getFirst("Authorization");
        return ("Bearer " + token).equals(header);
    }

    private void write(HttpExchange exchange, int status, JsonObject payload) throws IOException {
        if (payload.has("httpStatus")) {
            payload.remove("httpStatus");
        }
        byte[] raw = GSON.toJson(payload).getBytes(StandardCharsets.UTF_8);
        exchange.getResponseHeaders().set("Content-Type", "application/json; charset=utf-8");
        exchange.sendResponseHeaders(status, raw.length);
        exchange.getResponseBody().write(raw);
        exchange.close();
    }

    private JsonObject ok() {
        JsonObject result = new JsonObject();
        result.addProperty("ok", true);
        return result;
    }

    private JsonObject error(String code, String message) {
        JsonObject result = new JsonObject();
        result.addProperty("ok", false);
        result.addProperty("code", code);
        result.addProperty("error", message == null ? code : message);
        return result;
    }

    private JsonObject vecJson(double x, double y, double z) {
        JsonObject result = new JsonObject();
        result.addProperty("x", x);
        result.addProperty("y", y);
        result.addProperty("z", z);
        return result;
    }

    private JsonObject blockPosJson(BlockPos pos) {
        JsonObject result = new JsonObject();
        result.addProperty("x", pos.x);
        result.addProperty("y", pos.y);
        result.addProperty("z", pos.z);
        return result;
    }

    private BlockPos requireBlockPos(JsonObject body) {
        return new BlockPos(requireInt(body, "x"), requireInt(body, "y"), requireInt(body, "z"));
    }

    private BlockPos queryBlockPos(HttpExchange exchange, BlockPos fallback) {
        return new BlockPos(
                queryInt(exchange, "x", fallback.x),
                queryInt(exchange, "y", fallback.y),
                queryInt(exchange, "z", fallback.z)
        );
    }

    private int requireInt(JsonObject body, String key) {
        if (!body.has(key)) {
            throw new IllegalArgumentException("missing integer field: " + key);
        }
        return body.get(key).getAsInt();
    }

    private boolean optionalBool(JsonObject body, String key, boolean fallback) {
        return body.has(key) ? body.get(key).getAsBoolean() : fallback;
    }

    private int optionalInt(JsonObject body, String key, int fallback) {
        return body.has(key) ? body.get(key).getAsInt() : fallback;
    }

    private float optionalFloat(JsonObject body, String key, float fallback) {
        return body.has(key) ? body.get(key).getAsFloat() : fallback;
    }

    private String optionalString(JsonObject body, String key, String fallback) {
        if (!body.has(key)) {
            return fallback;
        }
        String value = body.get(key).getAsString().trim();
        return value.length() == 0 ? fallback : value;
    }

    private int queryInt(HttpExchange exchange, String key, int fallback) {
        String value = queryParam(exchange, key);
        if (value == null || value.trim().length() == 0) {
            return fallback;
        }
        return Integer.parseInt(value);
    }

    private boolean queryBool(HttpExchange exchange, String key, boolean fallback) {
        String value = queryParam(exchange, key);
        if (value == null || value.trim().length() == 0) {
            return fallback;
        }
        return value.equalsIgnoreCase("true") || value.equals("1") || value.equalsIgnoreCase("yes");
    }

    private Integer queryOptionalInt(HttpExchange exchange, String key) {
        String value = queryParam(exchange, key);
        if (value == null || value.trim().length() == 0 || "null".equalsIgnoreCase(value)) {
            return null;
        }
        return Integer.valueOf(Integer.parseInt(value));
    }

    private String queryString(HttpExchange exchange, String key, String fallback) {
        String value = queryParam(exchange, key);
        return value == null || value.trim().length() == 0 ? fallback : value.trim();
    }

    private String queryParam(HttpExchange exchange, String key) {
        String query = exchange.getRequestURI().getRawQuery();
        if (query == null || query.trim().length() == 0) {
            return null;
        }
        for (String part : query.split("&")) {
            String[] pieces = part.split("=", 2);
            if (pieces.length > 0 && key.equals(pieces[0])) {
                try {
                    return pieces.length == 2 ? URLDecoder.decode(pieces[1], "UTF-8") : "";
                } catch (Exception exc) {
                    return pieces.length == 2 ? pieces[1] : "";
                }
            }
        }
        return null;
    }

    private int configuredPort() {
        String configured = configuredValue("power.mine.agent.port", "POWER_MINE_AGENT_PORT", Integer.toString(DEFAULT_PORT));
        try {
            int port = Integer.parseInt(configured);
            if (port < 1 || port > 65535) {
                throw new NumberFormatException("port out of range");
            }
            return port;
        } catch (NumberFormatException exc) {
            System.err.println("[Power Mine Forge Agent] invalid port " + configured + ", using " + DEFAULT_PORT);
            return DEFAULT_PORT;
        }
    }

    private String configuredValue(String property, String environment, String fallback) {
        String value = System.getProperty(property);
        if (value != null && value.trim().length() > 0) {
            return value.trim();
        }
        value = System.getenv(environment);
        if (value != null && value.trim().length() > 0) {
            return value.trim();
        }
        return fallback;
    }

    private int clamp(int value, int min, int max) {
        return Math.max(min, Math.min(max, value));
    }

    private String joinNames(String[] names) {
        StringBuilder builder = new StringBuilder();
        for (int index = 0; index < names.length; index++) {
            if (index > 0) {
                builder.append(", ");
            }
            builder.append(names[index]);
        }
        return builder.toString();
    }

    private interface Route {
        JsonObject handle(HttpExchange exchange) throws Exception;
    }

    private interface ClientTask {
        JsonObject run() throws Exception;
    }

    private static final class ScreenshotCapture {
        final File path;
        final BufferedImage image;
        final int sampledPixels;
        final int nonZeroSamples;

        ScreenshotCapture(File path, BufferedImage image, int sampledPixels, int nonZeroSamples) {
            this.path = path;
            this.image = image;
            this.sampledPixels = sampledPixels;
            this.nonZeroSamples = nonZeroSamples;
        }
    }

    private static final class BlockPos {
        final int x;
        final int y;
        final int z;

        BlockPos(int x, int y, int z) {
            this.x = x;
            this.y = y;
            this.z = z;
        }
    }
}
