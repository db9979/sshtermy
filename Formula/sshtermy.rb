# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.344"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.344/sshtermy_darwin_arm64"
      sha256 "276126e2b1044b41ffeedec2c50a912a50ee360009dfe9c233a2574ce09116f8"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.344/sshtermy_darwin_amd64"
      sha256 "76e885a471cc2e344ad66aae0b9c88c0ca0abd2740e88ed6233f70c1efbefdfd"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.344/sshtermy_linux_arm64"
      sha256 "08a3256ede7178f05ed5b84f28daad66613e91594638b4c58f15e663e7989e0c"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.344/sshtermy_linux_amd64"
      sha256 "3938cfc0899a8a5f5ff3bb7249562b06785ae6a7c6f3c649ea5bd4c7ce828a12"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
