package dev.powermine.agent;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import net.fabricmc.api.ClientModInitializer;
import net.minecraft.block.Block;
import net.minecraft.block.BlockState;
import net.minecraft.client.MinecraftClient;
import net.minecraft.client.render.model.BakedModel;
import net.minecraft.client.render.model.json.ModelTransformationMode;
import net.minecraft.client.texture.NativeImage;
import net.minecraft.client.util.ScreenshotRecorder;
import net.minecraft.entity.player.PlayerInventory;
import net.minecraft.inventory.CraftingInventory;
import net.minecraft.item.Item;
import net.minecraft.item.ItemStack;
import net.minecraft.recipe.CraftingRecipe;
import net.minecraft.recipe.RecipeType;
import net.minecraft.registry.Registries;
import net.minecraft.server.MinecraftServer;
import net.minecraft.server.network.ServerPlayerEntity;
import net.minecraft.server.world.ServerWorld;
import net.minecraft.util.ActionResult;
import net.minecraft.util.Hand;
import net.minecraft.util.Identifier;
import net.minecraft.util.hit.BlockHitResult;
import net.minecraft.util.math.BlockPos;
import net.minecraft.util.math.Direction;
import net.minecraft.util.math.Vec3d;

import java.io.IOException;
import java.io.InputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.HashMap;
import java.util.Locale;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

public final class PowerMineAgentClient implements ClientModInitializer {
    private static final Gson GSON = new GsonBuilder().disableHtmlEscaping().create();
    private static final int DEFAULT_PORT = 39276;
    private static final int MAX_SNAPSHOT_RADIUS = 8;

    private HttpServer server;

    @Override
    public void onInitializeClient() {
        int port = configuredPort();
        String token = configuredValue("power.mine.agent.token", "POWER_MINE_AGENT_TOKEN", "");
        try {
            server = HttpServer.create(new InetSocketAddress("127.0.0.1", port), 0);
            register("GET", "/health", token, this::health);
            register("GET", "/bridge/capabilities", token, exchange -> bridgeCapabilities());
            register("GET", "/capabilities", token, exchange -> bridgeCapabilities());
            register("GET", "/state", token, exchange -> clientThread(this::state));
            register("GET", "/inventory", token, exchange -> clientThread(this::inventory));
            register("GET", "/world/snapshot", token, exchange -> clientThread(() -> worldSnapshot(exchange)));
            register("POST", "/world/open", token, exchange -> clientThread(() -> openWorld(parseBody(exchange))));
            register("GET", "/render/held-item", token, exchange -> clientThread(() -> heldItemRender(exchange)));
            register("GET", "/render/block", token, exchange -> clientThread(() -> blockRender(exchange)));
            register("GET", "/render/screenshot", token, exchange -> clientThread(this::screenshot));
            register("POST", "/camera/look", token, exchange -> cameraLook(parseBody(exchange)));
            register("POST", "/input/release", token, exchange -> clientThread(this::releaseInput));
            register("POST", "/inventory/give", token, exchange -> clientThread(() -> giveItem(parseBody(exchange))));
            register("POST", "/hotbar/select", token, exchange -> clientThread(() -> selectHotbar(parseBody(exchange))));
            register("POST", "/block/place", token, exchange -> clientThread(() -> placeBlock(parseBody(exchange))));
            register("POST", "/block/break", token, exchange -> clientThread(() -> breakBlock(parseBody(exchange))));
            register("POST", "/recipe/check", token, exchange -> clientThread(() -> recipeCheck(parseBody(exchange))));
            register("POST", "/recipe/craft", token, exchange -> clientThread(() -> craftRecipe(parseBody(exchange))));
            register("POST", "/tick/wait", token, exchange -> waitTicks(parseBody(exchange)));
            register("POST", "/item/use", token, exchange -> clientThread(() -> useItem(parseBody(exchange))));
            register("POST", "/block/use", token, exchange -> clientThread(() -> useBlock(parseBody(exchange))));
            server.setExecutor(Executors.newCachedThreadPool(runnable -> {
                Thread thread = new Thread(runnable, "Power Mine Agent HTTP");
                thread.setDaemon(true);
                return thread;
            }));
            server.start();
            disablePauseOnLostFocus(MinecraftClient.getInstance());
            System.out.println("[Power Mine Agent] listening on 127.0.0.1:" + port);
        } catch (IOException exc) {
            System.err.println("[Power Mine Agent] failed to start: " + exc.getMessage());
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
                write(exchange, 200, route.handle(exchange));
            } catch (IllegalArgumentException exc) {
                write(exchange, 400, error("bad_request", exc.getMessage()));
            } catch (Exception exc) {
                write(exchange, 500, error("agent_error", exc.getMessage()));
            }
        });
    }

    private JsonObject health(HttpExchange exchange) {
        JsonObject result = ok();
        result.addProperty("name", "power-mine-agent");
        result.addProperty("version", "0.2.0");
        result.addProperty("protocolVersion", "power-mine-agent/v1");
        result.addProperty("loader", "fabric");
        result.addProperty("minecraftVersion", "1.20.1");
        result.addProperty("minecraftThread", MinecraftClient.getInstance() != null);
        return result;
    }

    private JsonObject bridgeCapabilities() {
        JsonObject result = ok();
        result.addProperty("protocolVersion", "power-mine-agent/v1");
        result.addProperty("agentId", "power-mine-fabric-agent");
        result.addProperty("agentVersion", "0.2.0");
        result.addProperty("loader", "fabric");
        result.addProperty("minecraftVersion", "1.20.1");

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
        capabilities.addProperty("bakedModelIntrospection", true);
        capabilities.addProperty("cameraLook", true);
        capabilities.addProperty("pauseOnLostFocusControl", true);
        capabilities.addProperty("inputRelease", true);
        capabilities.addProperty("offhand", true);
        capabilities.addProperty("autoWorldOpen", true);
        capabilities.addProperty("autoWorldCreate", false);
        result.add("capabilities", capabilities);

        JsonArray limitations = new JsonArray();
        limitations.add("World and block mutation endpoints require an integrated singleplayer server.");
        limitations.add("The agent can open an existing local world from the title screen, but does not create new worlds yet.");
        limitations.add("The agent disables pause-on-lost-focus for automation sessions and can release the mouse cursor on request.");
        result.add("limitations", limitations);
        return result;
    }

    private JsonObject state() {
        MinecraftClient client = MinecraftClient.getInstance();
        JsonObject result = ok();
        result.addProperty("loaded", client.player != null && client.world != null);
        result.addProperty("singleplayer", client.getServer() != null);
        if (client.player == null || client.world == null) {
            return result;
        }

        result.addProperty("screen", client.currentScreen == null ? "" : client.currentScreen.getClass().getName());
        result.addProperty("dimension", client.world.getRegistryKey().getValue().toString());
        result.add("player", playerJson(client));
        result.add("target", targetJson(client));
        result.add("inventorySummary", inventorySummary(client.player.getInventory()));
        return result;
    }

    private JsonObject inventory() {
        MinecraftClient client = requireLoadedClient();
        JsonObject result = ok();
        PlayerInventory inventory = client.player.getInventory();
        result.addProperty("selectedSlot", inventory.selectedSlot);
        JsonArray slots = new JsonArray();
        for (int index = 0; index < inventory.size(); index++) {
            slots.add(stackJson(index, inventory.getStack(index), inventory.selectedSlot));
        }
        result.add("slots", slots);
        return result;
    }

    private JsonObject worldSnapshot(HttpExchange exchange) {
        MinecraftClient client = requireLoadedClient();
        int radius = clamp(queryInt(exchange, "radius", 2), 0, MAX_SNAPSHOT_RADIUS);
        boolean includeAir = queryBool(exchange, "includeAir", false);
        BlockPos center = queryBlockPos(exchange, client.player.getBlockPos());

        JsonObject result = ok();
        result.addProperty("dimension", client.world.getRegistryKey().getValue().toString());
        result.add("center", blockPosJson(center));
        result.addProperty("radius", radius);
        result.addProperty("includeAir", includeAir);

        JsonArray blocks = new JsonArray();
        for (int y = center.getY() - radius; y <= center.getY() + radius; y++) {
            for (int z = center.getZ() - radius; z <= center.getZ() + radius; z++) {
                for (int x = center.getX() - radius; x <= center.getX() + radius; x++) {
                    BlockPos pos = new BlockPos(x, y, z);
                    BlockState state = client.world.getBlockState(pos);
                    if (!includeAir && state.isAir()) {
                        continue;
                    }
                    JsonObject block = blockPosJson(pos);
                    block.addProperty("id", Registries.BLOCK.getId(state.getBlock()).toString());
                    block.addProperty("state", state.toString());
                    blocks.add(block);
                }
            }
        }
        result.add("blocks", blocks);
        return result;
    }

    private JsonObject openWorld(JsonObject body) {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null) {
            throw new IllegalStateException("Minecraft client is not ready");
        }
        disablePauseOnLostFocus(client);
        String worldName = optionalString(body, "world", optionalString(body, "name", ""));
        if (worldName.isBlank()) {
            throw new IllegalArgumentException("missing world name: pass world or name");
        }
        boolean create = optionalBool(body, "create", false);
        if (client.player != null && client.world != null) {
            JsonObject result = ok();
            result.addProperty("loaded", true);
            result.addProperty("alreadyLoaded", true);
            result.addProperty("world", worldName);
            return result;
        }
        if (create && !client.getLevelStorage().levelExists(worldName)) {
            throw new IllegalArgumentException("Fabric 1.20.1 agent can open existing worlds, but cannot create new worlds yet: " + worldName);
        }
        if (!client.getLevelStorage().levelExists(worldName)) {
            throw new IllegalArgumentException("world does not exist: " + worldName);
        }
        client.createIntegratedServerLoader().start(null, worldName);
        JsonObject result = ok();
        result.addProperty("loaded", false);
        result.addProperty("opening", true);
        result.addProperty("world", worldName);
        result.addProperty("create", false);
        return result;
    }

    private JsonObject releaseInput() {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null) {
            throw new IllegalStateException("Minecraft client is not ready");
        }
        boolean changed = disablePauseOnLostFocus(client);
        client.mouse.unlockCursor();

        JsonObject result = ok();
        result.addProperty("pauseOnLostFocus", client.options.pauseOnLostFocus);
        result.addProperty("pauseOnLostFocusChanged", changed);
        result.addProperty("mouseReleased", true);
        return result;
    }

    private boolean disablePauseOnLostFocus(MinecraftClient client) {
        if (client == null || client.options == null) {
            return false;
        }
        boolean previous = client.options.pauseOnLostFocus;
        client.options.pauseOnLostFocus = false;
        return previous;
    }

    private JsonObject heldItemRender(HttpExchange exchange) {
        MinecraftClient client = requireLoadedClient();
        String hand = queryString(exchange, "hand", "main").toLowerCase(Locale.ROOT);
        ItemStack stack;
        if (hand.equals("offhand") || hand.equals("off")) {
            stack = client.player.getOffHandStack();
            hand = "offhand";
        } else {
            stack = client.player.getMainHandStack();
            hand = "main";
        }

        JsonObject result = ok();
        result.addProperty("hand", hand);
        result.add("stack", stackJson(-1, stack, -1));
        result.add("render", itemRenderJson(client, stack));
        return result;
    }

    private JsonObject blockRender(HttpExchange exchange) throws Exception {
        MinecraftClient client = requireLoadedClient();
        BlockPos pos = queryBlockPos(exchange, client.player.getBlockPos());
        BlockState state = blockStateForRender(client, pos);
        BakedModel model = client.getBakedModelManager().getBlockModels().getModel(state);

        JsonObject result = ok();
        result.add("pos", blockPosJson(pos));
        result.addProperty("id", Registries.BLOCK.getId(state.getBlock()).toString());
        result.addProperty("state", state.toString());
        result.add("render", modelJson(client, model));
        return result;
    }

    private BlockState blockStateForRender(MinecraftClient client, BlockPos pos) throws Exception {
        MinecraftServer server = client.getServer();
        if (server == null) {
            return client.world.getBlockState(pos);
        }
        var worldKey = client.world.getRegistryKey();
        CompletableFuture<BlockState> future = new CompletableFuture<>();
        server.execute(() -> {
            try {
                ServerWorld world = server.getWorld(worldKey);
                if (world == null) {
                    future.complete(client.world.getBlockState(pos));
                } else {
                    future.complete(world.getBlockState(pos));
                }
            } catch (Throwable throwable) {
                future.completeExceptionally(throwable);
            }
        });
        return future.get(10, TimeUnit.SECONDS);
    }

    private JsonObject screenshot() throws IOException {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null) {
            throw new IllegalStateException("Minecraft client is not ready");
        }
        Path directory = client.runDirectory.toPath().resolve("power-mine-agent").resolve("screenshots");
        Files.createDirectories(directory);
        Path path = directory.resolve("screenshot-" + System.currentTimeMillis() + ".png");

        int width;
        int height;
        int sampledPixels = 0;
        int nonZeroSamples = 0;
        try (NativeImage image = ScreenshotRecorder.takeScreenshot(client.getFramebuffer())) {
            width = image.getWidth();
            height = image.getHeight();
            int stepX = Math.max(1, width / 32);
            int stepY = Math.max(1, height / 32);
            for (int y = 0; y < height; y += stepY) {
                for (int x = 0; x < width; x += stepX) {
                    sampledPixels++;
                    if (image.getColor(x, y) != 0) {
                        nonZeroSamples++;
                    }
                }
            }
            image.writeTo(path);
        }

        JsonObject result = ok();
        result.addProperty("path", path.toString());
        result.addProperty("width", width);
        result.addProperty("height", height);
        result.addProperty("sampledPixels", sampledPixels);
        result.addProperty("nonZeroSamples", nonZeroSamples);
        result.addProperty("blankLikely", nonZeroSamples == 0);
        return result;
    }

    private JsonObject cameraLook(JsonObject body) throws Exception {
        JsonObject result = clientThread(() -> {
            MinecraftClient client = requireLoadedClient();
            boolean hasTarget = body.has("x") || body.has("y") || body.has("z");
            float yaw;
            float pitch;

            JsonObject rotation = ok();
            if (hasTarget) {
                if (!body.has("x") || !body.has("y") || !body.has("z")) {
                    throw new IllegalArgumentException("x, y, and z are required when looking at a target");
                }
                boolean center = optionalBool(body, "center", true);
                double targetX = optionalDouble(body, "x", 0.0) + (center ? 0.5 : 0.0);
                double targetY = optionalDouble(body, "y", 0.0) + (center ? 0.5 : 0.0);
                double targetZ = optionalDouble(body, "z", 0.0) + (center ? 0.5 : 0.0);
                Vec3d eye = client.player.getEyePos();
                double dx = targetX - eye.x;
                double dy = targetY - eye.y;
                double dz = targetZ - eye.z;
                double horizontal = Math.sqrt(dx * dx + dz * dz);
                yaw = (float) (Math.atan2(dz, dx) * 180.0 / Math.PI) - 90.0f;
                pitch = (float) (-(Math.atan2(dy, horizontal) * 180.0 / Math.PI));

                JsonObject target = new JsonObject();
                target.addProperty("x", targetX);
                target.addProperty("y", targetY);
                target.addProperty("z", targetZ);
                target.addProperty("center", center);
                rotation.add("target", target);
            } else {
                if (!body.has("yaw") || !body.has("pitch")) {
                    throw new IllegalArgumentException("pass either x/y/z target fields or yaw and pitch");
                }
                yaw = (float) optionalDouble(body, "yaw", client.player.getYaw());
                pitch = (float) optionalDouble(body, "pitch", client.player.getPitch());
            }

            pitch = (float) Math.max(-90.0, Math.min(90.0, pitch));
            setCameraRotation(client, yaw, pitch);
            rotation.addProperty("yaw", yaw);
            rotation.addProperty("pitch", pitch);
            return rotation;
        });
        int delayMs = clamp(optionalInt(body, "delayMs", optionalInt(body, "delay_ms", 0)), 0, 2000);
        if (delayMs > 0) {
            Thread.sleep(delayMs);
        }
        if (optionalBool(body, "screenshot", false)) {
            result.add("screenshot", clientThread(this::screenshot));
        }
        return result;
    }

    private void setCameraRotation(MinecraftClient client, float yaw, float pitch) {
        client.player.setYaw(yaw);
        client.player.setPitch(pitch);
        client.player.setHeadYaw(yaw);
        client.player.bodyYaw = yaw;
        client.player.prevYaw = yaw;
        client.player.prevPitch = pitch;
        client.player.prevHeadYaw = yaw;
        client.player.prevBodyYaw = yaw;
    }

    private JsonObject selectHotbar(JsonObject body) {
        MinecraftClient client = requireLoadedClient();
        int slot = requireInt(body, "slot");
        if (slot < 0 || slot > 8) {
            throw new IllegalArgumentException("slot must be between 0 and 8");
        }
        client.player.getInventory().selectedSlot = slot;
        JsonObject result = ok();
        result.addProperty("selectedSlot", slot);
        return result;
    }

    private JsonObject giveItem(JsonObject body) throws Exception {
        MinecraftClient client = requireLoadedClient();
        MinecraftServer server = requireIntegratedServer(client);
        UUID playerUuid = client.player.getUuid();
        String itemId = optionalString(body, "item", optionalString(body, "id", ""));
        if (itemId.isBlank()) {
            throw new IllegalArgumentException("missing item id: pass item or id");
        }
        Optional<Item> item = Registries.ITEM.getOrEmpty(new Identifier(itemId));
        if (item.isEmpty()) {
            throw new IllegalArgumentException("unknown item id: " + itemId);
        }

        int slot = optionalInt(body, "slot", client.player.getInventory().selectedSlot);
        if (slot < 0 || slot >= client.player.getInventory().size()) {
            throw new IllegalArgumentException("slot must be between 0 and " + (client.player.getInventory().size() - 1));
        }
        int count = clamp(optionalInt(body, "count", 1), 1, item.get().getMaxCount());
        boolean select = optionalBool(body, "select", true);
        boolean replace = optionalBool(body, "replace", true);
        ItemStack stack = new ItemStack(item.get(), count);

        JsonObject result = serverThread(server, () -> {
            ServerPlayerEntity player = requireServerPlayer(server, playerUuid);
            PlayerInventory inventory = player.getInventory();
            ItemStack previous = inventory.getStack(slot).copy();
            if (!replace && !previous.isEmpty()) {
                throw new IllegalArgumentException("inventory slot is not empty: " + slot);
            }
            inventory.setStack(slot, stack.copy());
            if (select && slot >= 0 && slot <= 8) {
                inventory.selectedSlot = slot;
            }
            inventory.markDirty();
            player.currentScreenHandler.sendContentUpdates();

            JsonObject response = ok();
            response.addProperty("item", itemId);
            response.addProperty("count", count);
            response.addProperty("slot", slot);
            response.addProperty("selected", select && slot >= 0 && slot <= 8);
            response.add("previous", stackJson(slot, previous, inventory.selectedSlot));
            response.add("stack", stackJson(slot, inventory.getStack(slot), inventory.selectedSlot));
            return response;
        });

        client.player.getInventory().setStack(slot, stack.copy());
        if (select && slot >= 0 && slot <= 8) {
            client.player.getInventory().selectedSlot = slot;
            result.addProperty("selectedSlot", slot);
        }
        return result;
    }

    private JsonObject placeBlock(JsonObject body) throws Exception {
        MinecraftClient client = requireLoadedClient();
        MinecraftServer server = requireIntegratedServer(client);
        UUID playerUuid = client.player.getUuid();
        BlockPos pos = requireBlockPos(body);
        String blockId = optionalString(body, "block", "minecraft:stone");
        Optional<Block> block = Registries.BLOCK.getOrEmpty(new Identifier(blockId));
        if (block.isEmpty()) {
            throw new IllegalArgumentException("unknown block id: " + blockId);
        }

        return serverThread(server, () -> {
            ServerPlayerEntity player = requireServerPlayer(server, playerUuid);
            ServerWorld world = player.getServerWorld();
            boolean changed = world.setBlockState(pos, block.get().getDefaultState());
            JsonObject result = ok();
            result.addProperty("changed", changed);
            result.add("pos", blockPosJson(pos));
            result.addProperty("id", Registries.BLOCK.getId(world.getBlockState(pos).getBlock()).toString());
            return result;
        });
    }

    private JsonObject breakBlock(JsonObject body) throws Exception {
        MinecraftClient client = requireLoadedClient();
        MinecraftServer server = requireIntegratedServer(client);
        UUID playerUuid = client.player.getUuid();
        BlockPos pos = requireBlockPos(body);
        boolean drop = optionalBool(body, "drop", false);

        return serverThread(server, () -> {
            ServerPlayerEntity player = requireServerPlayer(server, playerUuid);
            ServerWorld world = player.getServerWorld();
            boolean changed = world.breakBlock(pos, drop, player);
            JsonObject result = ok();
            result.addProperty("changed", changed);
            result.add("pos", blockPosJson(pos));
            result.addProperty("id", Registries.BLOCK.getId(world.getBlockState(pos).getBlock()).toString());
            return result;
        });
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
        JsonObject result = clientThread(() -> {
            MinecraftClient client = requireLoadedClient();
            JsonObject response = ok();
            response.addProperty("worldTime", client.world.getTime());
            return response;
        });
        return result.get("worldTime").getAsLong();
    }

    private JsonObject useItem(JsonObject body) {
        MinecraftClient client = requireLoadedClient();
        if (client.interactionManager == null) {
            throw new IllegalStateException("Minecraft interaction manager is not ready");
        }
        if (body.has("slot")) {
            int slot = clamp(optionalInt(body, "slot", client.player.getInventory().selectedSlot), 0, 8);
            client.player.getInventory().selectedSlot = slot;
        }

        Hand hand = handFromString(optionalString(body, "hand", "main"));
        ItemStack before = stackForHand(client, hand).copy();
        ActionResult action = client.interactionManager.interactItem(client.player, hand);
        ItemStack after = stackForHand(client, hand).copy();

        JsonObject result = ok();
        result.addProperty("hand", hand == Hand.OFF_HAND ? "offhand" : "main");
        result.addProperty("selectedSlot", client.player.getInventory().selectedSlot);
        result.addProperty("actionResult", action.toString());
        result.addProperty("accepted", action.isAccepted());
        result.addProperty("screen", client.currentScreen == null ? "" : client.currentScreen.getClass().getName());
        result.add("before", stackJson(-1, before, -1));
        result.add("after", stackJson(-1, after, -1));
        return result;
    }

    private JsonObject useBlock(JsonObject body) {
        MinecraftClient client = requireLoadedClient();
        if (client.interactionManager == null) {
            throw new IllegalStateException("Minecraft interaction manager is not ready");
        }
        if (body.has("slot")) {
            int slot = clamp(optionalInt(body, "slot", client.player.getInventory().selectedSlot), 0, 8);
            client.player.getInventory().selectedSlot = slot;
        }

        BlockPos pos = requireBlockPos(body);
        Hand hand = handFromString(optionalString(body, "hand", "main"));
        Direction side = directionFromString(optionalString(body, "side", "up"));
        ItemStack beforeStack = stackForHand(client, hand).copy();
        BlockState beforeBlock = client.world.getBlockState(pos);
        BlockHitResult hit = new BlockHitResult(Vec3d.ofCenter(pos), side, pos, false);
        ActionResult action = client.interactionManager.interactBlock(client.player, hand, hit);
        ItemStack afterStack = stackForHand(client, hand).copy();
        BlockState afterBlock = client.world.getBlockState(pos);

        JsonObject result = ok();
        result.add("pos", blockPosJson(pos));
        result.addProperty("hand", hand == Hand.OFF_HAND ? "offhand" : "main");
        result.addProperty("side", side.asString());
        result.addProperty("selectedSlot", client.player.getInventory().selectedSlot);
        result.addProperty("actionResult", action.toString());
        result.addProperty("accepted", action.isAccepted());
        result.addProperty("screen", client.currentScreen == null ? "" : client.currentScreen.getClass().getName());
        result.add("beforeBlock", blockStateJson(pos, beforeBlock));
        result.add("afterBlock", blockStateJson(pos, afterBlock));
        result.add("beforeStack", stackJson(-1, beforeStack, -1));
        result.add("afterStack", stackJson(-1, afterStack, -1));
        return result;
    }

    private JsonObject recipeCheck(JsonObject body) throws Exception {
        MinecraftClient client = requireLoadedClient();
        MinecraftServer server = requireIntegratedServer(client);
        UUID playerUuid = client.player.getUuid();
        int width = clamp(optionalInt(body, "width", 3), 1, 3);
        int height = clamp(optionalInt(body, "height", 3), 1, 3);
        boolean requireInventory = optionalBool(body, "requireInventory", false);
        String expectedOutput = optionalString(body, "expectedOutput", "");
        String expectedRecipe = optionalString(body, "recipeId", "");
        JsonArray itemSpecs = body.has("items") && body.get("items").isJsonArray() ? body.getAsJsonArray("items") : new JsonArray();

        return serverThread(server, () -> {
            ServerPlayerEntity player = requireServerPlayer(server, playerUuid);
            CraftingInventory crafting = new CraftingInventory(player.currentScreenHandler, width, height);
            JsonArray grid = new JsonArray();
            for (int index = 0; index < width * height; index++) {
                ItemStack stack = stackFromGridSpec(itemSpecs.size() > index ? itemSpecs.get(index) : null);
                crafting.setStack(index, stack);
                grid.add(stackJson(index, stack, -1));
            }

            Optional<CraftingRecipe> match = server.getRecipeManager().getFirstMatch(RecipeType.CRAFTING, crafting, player.getWorld());
            boolean availableFromInventory = inventoryContains(player.getInventory(), crafting);
            JsonObject result = ok();
            result.addProperty("width", width);
            result.addProperty("height", height);
            result.add("grid", grid);
            result.addProperty("matched", match.isPresent());
            result.addProperty("requireInventory", requireInventory);
            result.addProperty("availableFromInventory", availableFromInventory);
            if (match.isPresent()) {
                CraftingRecipe recipe = match.get();
                ItemStack crafted = recipe.craft(crafting, server.getRegistryManager());
                result.addProperty("recipeId", recipe.getId().toString());
                result.add("output", stackJson(-1, crafted, -1));
                if (!expectedOutput.isBlank()) {
                    result.addProperty("expectedOutput", expectedOutput);
                    result.addProperty("expectedOutputMatches", expectedOutput.equals(Registries.ITEM.getId(crafted.getItem()).toString()));
                }
                if (!expectedRecipe.isBlank()) {
                    result.addProperty("expectedRecipe", expectedRecipe);
                    result.addProperty("expectedRecipeMatches", expectedRecipe.equals(recipe.getId().toString()));
                }
            }
            result.addProperty("craftable", match.isPresent() && (!requireInventory || availableFromInventory));
            return result;
        });
    }

    private JsonObject craftRecipe(JsonObject body) throws Exception {
        MinecraftClient client = requireLoadedClient();
        MinecraftServer server = requireIntegratedServer(client);
        UUID playerUuid = client.player.getUuid();
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

        JsonObject result = serverThread(server, () -> {
            ServerPlayerEntity player = requireServerPlayer(server, playerUuid);
            PlayerInventory inventory = player.getInventory();
            CraftingInventory crafting = new CraftingInventory(player.currentScreenHandler, width, height);
            JsonArray grid = new JsonArray();
            for (int index = 0; index < width * height; index++) {
                ItemStack stack = stackFromGridSpec(itemSpecs.size() > index ? itemSpecs.get(index) : null);
                crafting.setStack(index, stack);
                grid.add(stackJson(index, stack, -1));
            }

            Optional<CraftingRecipe> match = server.getRecipeManager().getFirstMatch(RecipeType.CRAFTING, crafting, player.getWorld());
            JsonObject response = ok();
            response.addProperty("width", width);
            response.addProperty("height", height);
            response.addProperty("crafts", crafts);
            response.addProperty("requireInventory", requireInventory);
            response.addProperty("consume", consume);
            response.addProperty("insertOutput", insertOutput);
            response.add("grid", grid);
            response.addProperty("matched", match.isPresent());
            if (expectedOutput != null && !expectedOutput.isBlank()) {
                response.addProperty("expectedOutput", expectedOutput);
            }

            if (match.isEmpty()) {
                response.addProperty("ok", false);
                response.addProperty("crafted", false);
                response.addProperty("error", "no matching recipe");
                return response;
            }

            CraftingRecipe recipe = match.get();
            ItemStack crafted = recipe.craft(crafting, server.getRegistryManager());
            if (crafted.isEmpty()) {
                response.addProperty("ok", false);
                response.addProperty("crafted", false);
                response.addProperty("recipeId", recipe.getId().toString());
                response.addProperty("error", "matched recipe produced an empty output");
                return response;
            }

            response.addProperty("recipeId", recipe.getId().toString());
            response.add("output", stackJson(-1, crafted, -1));
            if (expectedOutput != null && !expectedOutput.isBlank()) {
                response.addProperty("expectedOutputMatches", expectedOutput.equals(Registries.ITEM.getId(crafted.getItem()).toString()));
            }

            Map<String, Integer> required = requiredStacks(crafting, crafts);
            response.add("requiredItems", stackRequirementJson(required));
            JsonArray missing = missingItems(inventory, required);
            response.add("missingItems", missing);
            boolean availableFromInventory = missing.size() == 0;
            response.addProperty("availableFromInventory", availableFromInventory);
            if (requireInventory && !availableFromInventory) {
                response.addProperty("ok", false);
                response.addProperty("crafted", false);
                response.addProperty("error", "missing required ingredients");
                return response;
            }

            ItemStack output = crafted.copy();
            int outputCount = crafted.getCount() * crafts;
            if (outputCount > output.getMaxCount()) {
                response.addProperty("ok", false);
                response.addProperty("crafted", false);
                response.addProperty("error", "crafted output does not fit in one stack: " + outputCount);
                return response;
            }
            output.setCount(outputCount);

            if (consume && !required.isEmpty()) {
                JsonArray consumed = new JsonArray();
                consumeInventory(inventory, required, consumed);
                response.add("consumedItems", consumed);
            } else {
                response.add("consumedItems", new JsonArray());
            }

            int insertedSlot = -1;
            if (insertOutput) {
                insertedSlot = insertInventoryStack(inventory, output, outputSlot, replaceOutput);
                if (insertedSlot < 0) {
                    response.addProperty("ok", false);
                    response.addProperty("crafted", false);
                    response.addProperty("error", insertedSlot == -2 ? "output slot is not empty" : "no empty inventory slot for crafted output");
                    return response;
                }
            }

            inventory.markDirty();
            player.currentScreenHandler.sendContentUpdates();
            response.addProperty("crafted", true);
            response.addProperty("outputSlot", insertedSlot);
            response.add("craftedStack", stackJson(insertedSlot, output, inventory.selectedSlot));
            return response;
        });
        return result;
    }

    private JsonObject playerJson(MinecraftClient client) {
        JsonObject player = new JsonObject();
        player.addProperty("name", client.player.getName().getString());
        player.addProperty("uuid", client.player.getUuid().toString());
        player.add("pos", vecJson(client.player.getPos()));
        player.add("blockPos", blockPosJson(client.player.getBlockPos()));
        player.addProperty("yaw", client.player.getYaw());
        player.addProperty("pitch", client.player.getPitch());
        player.addProperty("health", client.player.getHealth());
        player.addProperty("food", client.player.getHungerManager().getFoodLevel());
        player.addProperty("selectedSlot", client.player.getInventory().selectedSlot);
        return player;
    }

    private JsonObject targetJson(MinecraftClient client) {
        JsonObject target = new JsonObject();
        if (client.crosshairTarget == null) {
            target.addProperty("type", "none");
            return target;
        }
        target.addProperty("type", client.crosshairTarget.getType().name().toLowerCase(Locale.ROOT));
        if (client.crosshairTarget instanceof net.minecraft.util.hit.BlockHitResult blockHit) {
            BlockPos pos = blockHit.getBlockPos();
            BlockState state = client.world.getBlockState(pos);
            Direction side = blockHit.getSide();
            target.add("pos", blockPosJson(pos));
            target.addProperty("side", side.asString());
            target.addProperty("id", Registries.BLOCK.getId(state.getBlock()).toString());
            target.addProperty("state", state.toString());
        }
        return target;
    }

    private JsonObject inventorySummary(PlayerInventory inventory) {
        JsonObject summary = new JsonObject();
        int occupied = 0;
        for (int index = 0; index < inventory.size(); index++) {
            if (!inventory.getStack(index).isEmpty()) {
                occupied++;
            }
        }
        summary.addProperty("slots", inventory.size());
        summary.addProperty("occupied", occupied);
        summary.addProperty("selectedSlot", inventory.selectedSlot);
        return summary;
    }

    private JsonObject blockStateJson(BlockPos pos, BlockState state) {
        JsonObject result = blockPosJson(pos);
        result.addProperty("id", Registries.BLOCK.getId(state.getBlock()).toString());
        result.addProperty("state", state.toString());
        return result;
    }

    private JsonObject stackJson(int index, ItemStack stack, int selectedSlot) {
        JsonObject item = new JsonObject();
        item.addProperty("slot", index);
        item.addProperty("slotType", slotType(index));
        item.addProperty("selected", index == selectedSlot);
        item.addProperty("empty", stack.isEmpty());
        if (!stack.isEmpty()) {
            item.addProperty("id", Registries.ITEM.getId(stack.getItem()).toString());
            item.addProperty("name", stack.getName().getString());
            item.addProperty("count", stack.getCount());
            item.addProperty("damage", stack.getDamage());
            item.addProperty("maxDamage", stack.getMaxDamage());
        }
        return item;
    }

    private ItemStack stackForHand(MinecraftClient client, Hand hand) {
        return hand == Hand.OFF_HAND ? client.player.getOffHandStack() : client.player.getMainHandStack();
    }

    private Hand handFromString(String value) {
        String normalized = value == null ? "" : value.trim().toLowerCase(Locale.ROOT);
        if (normalized.equals("off") || normalized.equals("offhand") || normalized.equals("off_hand")) {
            return Hand.OFF_HAND;
        }
        return Hand.MAIN_HAND;
    }

    private Direction directionFromString(String value) {
        String normalized = value == null ? "" : value.trim().toLowerCase(Locale.ROOT);
        switch (normalized) {
            case "down":
            case "0":
                return Direction.DOWN;
            case "north":
            case "2":
                return Direction.NORTH;
            case "south":
            case "3":
                return Direction.SOUTH;
            case "west":
            case "4":
                return Direction.WEST;
            case "east":
            case "5":
                return Direction.EAST;
            case "up":
            case "1":
            default:
                return Direction.UP;
        }
    }

    private JsonObject itemRenderJson(MinecraftClient client, ItemStack stack) {
        JsonObject result = new JsonObject();
        result.addProperty("empty", stack.isEmpty());
        if (stack.isEmpty()) {
            return result;
        }
        BakedModel model = client.getItemRenderer().getModel(stack, client.world, client.player, 0);
        result.add("model", modelJson(client, model));
        return result;
    }

    private JsonObject modelJson(MinecraftClient client, BakedModel model) {
        BakedModel missing = client.getBakedModelManager().getMissingModel();
        JsonObject result = new JsonObject();
        result.addProperty("missingModel", model == missing);
        result.addProperty("builtin", model.isBuiltin());
        result.addProperty("hasDepth", model.hasDepth());
        result.addProperty("sideLit", model.isSideLit());
        result.addProperty("ambientOcclusion", model.useAmbientOcclusion());
        result.addProperty("firstPersonRightHand", model.getTransformation().isTransformationDefined(ModelTransformationMode.FIRST_PERSON_RIGHT_HAND));
        result.addProperty("firstPersonLeftHand", model.getTransformation().isTransformationDefined(ModelTransformationMode.FIRST_PERSON_LEFT_HAND));
        result.addProperty("thirdPersonRightHand", model.getTransformation().isTransformationDefined(ModelTransformationMode.THIRD_PERSON_RIGHT_HAND));
        result.addProperty("gui", model.getTransformation().isTransformationDefined(ModelTransformationMode.GUI));
        result.add("particleSprite", spriteJson(model));
        return result;
    }

    private JsonObject spriteJson(BakedModel model) {
        JsonObject sprite = new JsonObject();
        if (model.getParticleSprite() == null) {
            sprite.addProperty("missing", true);
            return sprite;
        }
        sprite.addProperty("missing", false);
        sprite.addProperty("atlas", model.getParticleSprite().getAtlasId().toString());
        sprite.addProperty("id", model.getParticleSprite().getContents().getId().toString());
        sprite.addProperty("width", model.getParticleSprite().getContents().getWidth());
        sprite.addProperty("height", model.getParticleSprite().getContents().getHeight());
        return sprite;
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

    private ItemStack stackFromGridSpec(JsonElement element) {
        if (element == null || element.isJsonNull()) {
            return ItemStack.EMPTY;
        }
        String itemId;
        int count = 1;
        if (element.isJsonPrimitive()) {
            itemId = element.getAsString();
        } else if (element.isJsonObject()) {
            JsonObject object = element.getAsJsonObject();
            itemId = optionalString(object, "id", "");
            count = optionalInt(object, "count", 1);
        } else {
            throw new IllegalArgumentException("crafting grid entries must be strings or objects");
        }
        itemId = itemId == null ? "" : itemId.trim();
        if (itemId.isBlank() || itemId.equals("minecraft:air")) {
            return ItemStack.EMPTY;
        }
        Optional<Item> item = Registries.ITEM.getOrEmpty(new Identifier(itemId));
        if (item.isEmpty()) {
            throw new IllegalArgumentException("unknown item id: " + itemId);
        }
        return new ItemStack(item.get(), Math.max(1, count));
    }

    private boolean inventoryContains(PlayerInventory inventory, CraftingInventory crafting) {
        Map<String, Integer> required = new HashMap<>();
        for (int gridIndex = 0; gridIndex < crafting.size(); gridIndex++) {
            ItemStack stack = crafting.getStack(gridIndex);
            if (stack.isEmpty()) {
                continue;
            }
            String key = stackKey(stack);
            required.put(key, required.getOrDefault(key, 0) + stack.getCount());
        }

        Map<String, Integer> available = new HashMap<>();
        for (int slot = 0; slot < inventory.size(); slot++) {
            ItemStack stack = inventory.getStack(slot);
            if (stack.isEmpty()) {
                continue;
            }
            String key = stackKey(stack);
            available.put(key, available.getOrDefault(key, 0) + stack.getCount());
        }

        for (Map.Entry<String, Integer> entry : required.entrySet()) {
            if (available.getOrDefault(entry.getKey(), 0) < entry.getValue()) {
                return false;
            }
        }
        return true;
    }

    private Map<String, Integer> requiredStacks(CraftingInventory crafting, int crafts) {
        Map<String, Integer> required = new HashMap<>();
        for (int gridIndex = 0; gridIndex < crafting.size(); gridIndex++) {
            ItemStack stack = crafting.getStack(gridIndex);
            if (stack.isEmpty()) {
                continue;
            }
            String key = stackKey(stack);
            required.put(key, required.getOrDefault(key, 0) + (stack.getCount() * crafts));
        }
        return required;
    }

    private Map<String, Integer> inventoryCounts(PlayerInventory inventory) {
        Map<String, Integer> available = new HashMap<>();
        int limit = Math.min(36, inventory.size());
        for (int slot = 0; slot < limit; slot++) {
            ItemStack stack = inventory.getStack(slot);
            if (stack.isEmpty()) {
                continue;
            }
            String key = stackKey(stack);
            available.put(key, available.getOrDefault(key, 0) + stack.getCount());
        }
        return available;
    }

    private JsonArray stackRequirementJson(Map<String, Integer> required) {
        JsonArray items = new JsonArray();
        for (Map.Entry<String, Integer> entry : required.entrySet()) {
            JsonObject item = stackKeyJson(entry.getKey());
            item.addProperty("count", entry.getValue());
            items.add(item);
        }
        return items;
    }

    private JsonArray missingItems(PlayerInventory inventory, Map<String, Integer> required) {
        Map<String, Integer> available = inventoryCounts(inventory);
        JsonArray missing = new JsonArray();
        for (Map.Entry<String, Integer> entry : required.entrySet()) {
            int availableCount = available.getOrDefault(entry.getKey(), 0);
            if (availableCount >= entry.getValue()) {
                continue;
            }
            JsonObject item = stackKeyJson(entry.getKey());
            item.addProperty("required", entry.getValue());
            item.addProperty("available", availableCount);
            item.addProperty("missing", entry.getValue() - availableCount);
            missing.add(item);
        }
        return missing;
    }

    private void consumeInventory(PlayerInventory inventory, Map<String, Integer> required, JsonArray consumed) {
        Map<String, Integer> remaining = new HashMap<>(required);
        int limit = Math.min(36, inventory.size());
        for (int slot = 0; slot < limit; slot++) {
            ItemStack stack = inventory.getStack(slot);
            if (stack.isEmpty()) {
                continue;
            }
            String key = stackKey(stack);
            int needed = remaining.getOrDefault(key, 0);
            if (needed <= 0) {
                continue;
            }
            int taken = Math.min(needed, stack.getCount());
            JsonObject entry = stackJson(slot, stack.copy(), inventory.selectedSlot);
            entry.addProperty("consumed", taken);
            consumed.add(entry);
            stack.decrement(taken);
            if (stack.isEmpty()) {
                inventory.setStack(slot, ItemStack.EMPTY);
            }
            int left = needed - taken;
            if (left <= 0) {
                remaining.remove(key);
            } else {
                remaining.put(key, left);
            }
        }
    }

    private int insertInventoryStack(PlayerInventory inventory, ItemStack output, int outputSlot, boolean replace) {
        if (outputSlot >= 0) {
            if (outputSlot >= inventory.size()) {
                throw new IllegalArgumentException("output slot must be between 0 and " + (inventory.size() - 1));
            }
            ItemStack previous = inventory.getStack(outputSlot);
            if (!replace && !previous.isEmpty()) {
                return -2;
            }
            inventory.setStack(outputSlot, output.copy());
            return outputSlot;
        }

        int limit = Math.min(36, inventory.size());
        for (int slot = 0; slot < limit; slot++) {
            if (inventory.getStack(slot).isEmpty()) {
                inventory.setStack(slot, output.copy());
                return slot;
            }
        }
        return -1;
    }

    private JsonObject stackKeyJson(String key) {
        String[] pieces = key.split("\\|", 2);
        JsonObject item = new JsonObject();
        item.addProperty("key", key);
        item.addProperty("id", pieces.length > 0 ? pieces[0] : key);
        if (pieces.length > 1 && !pieces[1].isBlank()) {
            item.addProperty("nbt", pieces[1]);
        }
        return item;
    }

    private String stackKey(ItemStack stack) {
        return Registries.ITEM.getId(stack.getItem()) + "|" + (stack.hasNbt() ? stack.getNbt().toString() : "");
    }

    private MinecraftClient requireLoadedClient() {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null || client.player == null || client.world == null) {
            throw new IllegalStateException("Minecraft client is not in a loaded world");
        }
        return client;
    }

    private MinecraftServer requireIntegratedServer(MinecraftClient client) {
        MinecraftServer server = client.getServer();
        if (server == null) {
            throw new IllegalStateException("block actions are only available in integrated singleplayer worlds");
        }
        return server;
    }

    private ServerPlayerEntity requireServerPlayer(MinecraftServer server, UUID uuid) {
        ServerPlayerEntity player = server.getPlayerManager().getPlayer(uuid);
        if (player == null) {
            throw new IllegalStateException("server-side player is not ready");
        }
        return player;
    }

    private JsonObject clientThread(ClientTask task) throws Exception {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null) {
            throw new IllegalStateException("Minecraft client is not ready");
        }
        CompletableFuture<JsonObject> future = new CompletableFuture<>();
        client.execute(() -> {
            try {
                future.complete(task.run());
            } catch (Throwable throwable) {
                future.completeExceptionally(throwable);
            }
        });
        return future.get(10, TimeUnit.SECONDS);
    }

    private JsonObject serverThread(MinecraftServer server, ServerTask task) throws Exception {
        CompletableFuture<JsonObject> future = new CompletableFuture<>();
        server.execute(() -> {
            try {
                future.complete(task.run());
            } catch (Throwable throwable) {
                future.completeExceptionally(throwable);
            }
        });
        return future.get(10, TimeUnit.SECONDS);
    }

    private JsonObject parseBody(HttpExchange exchange) throws IOException {
        try (InputStream stream = exchange.getRequestBody()) {
            byte[] raw = stream.readAllBytes();
            if (raw.length == 0) {
                return new JsonObject();
            }
            JsonElement element = JsonParser.parseString(new String(raw, StandardCharsets.UTF_8));
            if (!element.isJsonObject()) {
                throw new IllegalArgumentException("request body must be a JSON object");
            }
            return element.getAsJsonObject();
        }
    }

    private boolean authorized(HttpExchange exchange, String token) {
        if (token == null || token.isBlank()) {
            return true;
        }
        String header = exchange.getRequestHeaders().getFirst("Authorization");
        return ("Bearer " + token).equals(header);
    }

    private void write(HttpExchange exchange, int status, JsonObject payload) throws IOException {
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

    private JsonObject vecJson(Vec3d vec) {
        JsonObject result = new JsonObject();
        result.addProperty("x", vec.x);
        result.addProperty("y", vec.y);
        result.addProperty("z", vec.z);
        return result;
    }

    private JsonObject blockPosJson(BlockPos pos) {
        JsonObject result = new JsonObject();
        result.addProperty("x", pos.getX());
        result.addProperty("y", pos.getY());
        result.addProperty("z", pos.getZ());
        return result;
    }

    private BlockPos requireBlockPos(JsonObject body) {
        return new BlockPos(requireInt(body, "x"), requireInt(body, "y"), requireInt(body, "z"));
    }

    private BlockPos queryBlockPos(HttpExchange exchange, BlockPos fallback) {
        return new BlockPos(
                queryInt(exchange, "x", fallback.getX()),
                queryInt(exchange, "y", fallback.getY()),
                queryInt(exchange, "z", fallback.getZ())
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

    private double optionalDouble(JsonObject body, String key, double fallback) {
        return body.has(key) ? body.get(key).getAsDouble() : fallback;
    }

    private String optionalString(JsonObject body, String key, String fallback) {
        if (!body.has(key)) {
            return fallback;
        }
        String value = body.get(key).getAsString().trim();
        return value.isEmpty() ? fallback : value;
    }

    private int queryInt(HttpExchange exchange, String key, int fallback) {
        String value = queryParam(exchange, key);
        if (value == null || value.isBlank()) {
            return fallback;
        }
        return Integer.parseInt(value);
    }

    private boolean queryBool(HttpExchange exchange, String key, boolean fallback) {
        String value = queryParam(exchange, key);
        if (value == null || value.isBlank()) {
            return fallback;
        }
        return value.equalsIgnoreCase("true") || value.equals("1") || value.equalsIgnoreCase("yes");
    }

    private String queryString(HttpExchange exchange, String key, String fallback) {
        String value = queryParam(exchange, key);
        if (value == null || value.isBlank()) {
            return fallback;
        }
        return value.trim();
    }

    private String queryParam(HttpExchange exchange, String key) {
        String query = exchange.getRequestURI().getRawQuery();
        if (query == null || query.isBlank()) {
            return null;
        }
        for (String part : query.split("&")) {
            String[] pieces = part.split("=", 2);
            if (pieces.length > 0 && key.equals(pieces[0])) {
                return pieces.length == 2 ? pieces[1] : "";
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
            System.err.println("[Power Mine Agent] invalid port " + configured + ", using " + DEFAULT_PORT);
            return DEFAULT_PORT;
        }
    }

    private String configuredValue(String property, String environment, String fallback) {
        String value = System.getProperty(property);
        if (value != null && !value.isBlank()) {
            return value.trim();
        }
        value = System.getenv(environment);
        if (value != null && !value.isBlank()) {
            return value.trim();
        }
        return fallback;
    }

    private int clamp(int value, int min, int max) {
        return Math.max(min, Math.min(max, value));
    }

    private interface Route {
        JsonObject handle(HttpExchange exchange) throws Exception;
    }

    private interface ClientTask {
        JsonObject run() throws Exception;
    }

    private interface ServerTask {
        JsonObject run() throws Exception;
    }
}
